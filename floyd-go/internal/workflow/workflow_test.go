package workflow

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"

	"github.com/stretchr/testify/require"
)

func TestCheckpointRestartAndFiltering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join(root, "personal")
	s := NewCheckpointService(root, state)
	c := NewCheckpoint("feature", "feature", []Step{{ID: "analyze", Status: StepStatusCompleted}, {ID: "approve", Type: StepTypeApprove, Status: StepStatusAwaiting}})
	c.Status = StatusPaused
	c.CurrentStep = 1
	c.Context["request"] = "Keep my input"
	require.NoError(t, s.Save(c))
	reopened := NewCheckpointService(root, state)
	got, err := reopened.Load(c.ID)
	require.NoError(t, err)
	require.Equal(t, c.ID, got.ID)
	require.Equal(t, StatusPaused, got.Status)
	require.Equal(t, c.Context, got.Context)
	all, err := reopened.List("feature")
	require.NoError(t, err)
	require.Len(t, all, 1)
	latest, err := reopened.GetLatest("feature")
	require.NoError(t, err)
	require.Equal(t, c.ID, latest.ID)
	empty, err := reopened.List("different")
	require.NoError(t, err)
	require.Empty(t, empty)
	_, err = reopened.Load("absent")
	require.ErrorContains(t, err, "not found")
}

func TestWorkflowRestoresCompletedDependencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEngine(root, filepath.Join(root, "personal"))
	c := NewCheckpoint("resume", "resume", []Step{{ID: "approved", Type: StepTypeApprove, Status: StepStatusCompleted}, {ID: "next", Type: StepTypeExecute, Status: StepStatusPending, Requires: []string{"approved"}}})
	c.CurrentStep = 1
	c.Status = StatusRunning
	e.executeWorkflow(context.Background(), c)
	got, err := e.Status(c.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, got.Status)
	require.Equal(t, StepStatusCompleted, got.Steps[1].Status)
}

func TestBlockedWorkflowDoesNotClaimCompletion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEngine(root, filepath.Join(root, "personal"))
	c := NewCheckpoint("blocked", "blocked", []Step{{ID: "work", Type: StepTypeExecute, Status: StepStatusPending, Requires: []string{"missing"}}})
	c.Status = StatusRunning
	e.executeWorkflow(context.Background(), c)
	got, err := e.Status(c.ID)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, got.Status)
	require.NotEmpty(t, got.Error)
	require.NotEqual(t, StepStatusCompleted, got.Steps[0].Status)
}

func TestDefaultExecutorDoesNotInventCommandSuccess(t *testing.T) {
	t.Parallel()
	e := NewDefaultStepExecutor()
	step := &Step{ID: "command", Type: StepTypeExecute, Command: "printf example"}
	result, err := e.Execute(context.Background(), step)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Error)
	require.NotEqual(t, StepStatusCompleted, step.Status)
	approval := &Step{ID: "approve", Type: StepTypeApprove}
	result, err = e.Execute(context.Background(), approval)
	require.NoError(t, err)
	require.True(t, result.NeedsApproval)
	require.False(t, result.Success)
	require.Equal(t, StepStatusAwaiting, approval.Status)
	valid, err := e.Validate(context.Background(), &Step{Validation: "command-not-run"})
	require.False(t, valid)
	require.Error(t, err)
}

func TestWorkflowApprovalResumeAfterRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join(root, "personal")
	e := NewEngine(root, state)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer e.broker.Shutdown()
	events := e.Subscribe(ctx)
	started, err := e.Start(ctx, FeatureImplementation(), map[string]string{"request": "preserve this"})
	require.NoError(t, err)
	paused := awaitWorkflowStatus(t, events, StatusPaused)
	require.Equal(t, started.ID, paused.ID)
	require.Equal(t, StepStatusAwaiting, paused.Steps[2].Status)
	require.Equal(t, StepStatusPending, started.Steps[0].Status, "returned start snapshot must not change while worker runs")
	restarted := NewEngine(root, state)
	defer restarted.broker.Shutdown()
	resumedEvents := restarted.Subscribe(ctx)
	resumed, err := restarted.Approve(ctx, started.ID, true)
	require.NoError(t, err)
	require.Equal(t, StatusRunning, resumed.Status)
	completed := awaitWorkflowStatus(t, resumedEvents, StatusCompleted)
	for _, step := range completed.Steps {
		require.Equal(t, StepStatusCompleted, step.Status, step.ID)
	}
	require.True(t, paused.Steps[0].CompletedAt.Equal(*completed.Steps[0].CompletedAt), "completed work must not rerun")
	require.Equal(t, "preserve this", completed.Context["request"])
	durable, err := restarted.Status(started.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, durable.Status)
	_, err = restarted.Resume(ctx, started.ID)
	require.ErrorContains(t, err, "not paused")
}

func awaitWorkflowStatus(t *testing.T, events <-chan pubsub.Event[Checkpoint], want WorkflowStatus) Checkpoint {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Payload.Status == want {
				return event.Payload
			}
		case <-timer.C:
			t.Fatalf("workflow never reached %s", want)
			return Checkpoint{}
		}
	}
}

type waitingExecutor struct {
	started chan struct{}
	calls   atomic.Int32
}

func (e *waitingExecutor) Execute(ctx context.Context, step *Step) (*StepResult, error) {
	e.calls.Add(1)
	close(e.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func (e *waitingExecutor) Rollback(context.Context, *Step) error         { return nil }
func (e *waitingExecutor) Validate(context.Context, *Step) (bool, error) { return true, nil }

func TestWorkflowCancelStopsWorkerAndProtectsSnapshots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := NewEngine(root, filepath.Join(root, "personal"))
	defer e.broker.Shutdown()
	executor := &waitingExecutor{started: make(chan struct{})}
	e.executor = executor
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started, err := e.Start(ctx, &Definition{Name: "cancel", Steps: []*Step{{ID: "running", Type: StepTypeExecute}, {ID: "must-not-run", Type: StepTypeExecute}}}, nil)
	require.NoError(t, err)
	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("executor did not start")
	}
	snapshot := e.GetActive()
	require.NotNil(t, snapshot)
	snapshot.Steps[0].ID = "caller mutation"
	require.Equal(t, "running", e.GetActive().Steps[0].ID)
	_, err = e.Start(ctx, CodeReview(), nil)
	require.ErrorContains(t, err, "already running")
	cancelled, err := e.Cancel(ctx, started.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, cancelled.Status)
	require.Equal(t, int32(1), executor.calls.Load())
	require.Nil(t, e.GetActive())
	durable, err := e.Status(started.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, durable.Status)
	require.Equal(t, StepStatusPending, durable.Steps[1].Status)
}

func TestCheckpointRejectsTraversalAndRemainsReadableDuringSave(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := NewCheckpointService(root, filepath.Join(root, "personal"))
	for _, id := range []string{"", "..", "../outside", `..\outside`} {
		c := NewCheckpoint("fixture", "fixture", nil)
		c.ID = id
		require.ErrorContains(t, s.Save(c), "invalid checkpoint ID")
		_, err := s.Load(id)
		require.ErrorContains(t, err, "invalid checkpoint ID")
	}
	c := NewCheckpoint("fixture", "fixture", nil)
	require.NoError(t, s.Save(c))
	done := make(chan error, 1)
	go func() {
		for range 30 {
			if err := s.Save(c); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for range 30 {
		got, err := s.Load(c.ID)
		require.NoError(t, err, "readers must never see a truncated checkpoint")
		require.Equal(t, c.ID, got.ID)
	}
	require.NoError(t, <-done)
}

type rollbackExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *rollbackExecutor) Execute(_ context.Context, step *Step) (*StepResult, error) {
	if step.ID == "completed" {
		return &StepResult{Success: true}, nil
	}
	return &StepResult{Error: "fixture step failed"}, nil
}
func (e *rollbackExecutor) Validate(context.Context, *Step) (bool, error) { return true, nil }
func (e *rollbackExecutor) Rollback(ctx context.Context, _ *Step) error {
	close(e.started)
	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestWorkflowOwnsWorkerUntilRollbackFinishes(t *testing.T) {
	t.Parallel()
	for _, cancelRollback := range []bool{false, true} {
		name := "complete rollback"
		if cancelRollback {
			name = "cancel rollback"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			e := NewEngine(root, filepath.Join(root, "personal"))
			defer e.broker.Shutdown()
			executor := &rollbackExecutor{started: make(chan struct{}), release: make(chan struct{})}
			e.executor = executor
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			events := e.Subscribe(ctx)
			started, err := e.Start(ctx, &Definition{Name: "rollback", Steps: []*Step{{ID: "completed", Type: StepTypeExecute, RollbackCmd: "fixture-only"}, {ID: "failure", Type: StepTypeExecute}}}, nil)
			require.NoError(t, err)
			select {
			case <-executor.started:
			case <-ctx.Done():
				t.Fatal("rollback never started")
			}
			active := e.GetActive()
			require.NotNil(t, active)
			require.Equal(t, StatusRollback, active.Status)
			_, err = e.Start(ctx, CodeReview(), nil)
			require.ErrorContains(t, err, "already running")
			if cancelRollback {
				result, err := e.Cancel(ctx, started.ID)
				require.NoError(t, err)
				require.Equal(t, StatusCancelled, result.Status)
			} else {
				close(executor.release)
				result := awaitWorkflowStatus(t, events, StatusFailed)
				require.Contains(t, result.Error, "fixture step failed")
			}
			e.executor = NewDefaultStepExecutor()
			_, err = e.Start(ctx, CodeReview(), nil)
			require.NoError(t, err)
			awaitWorkflowStatus(t, events, StatusCompleted)
		})
	}
}
