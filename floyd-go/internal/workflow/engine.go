package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"
)

// WorkflowStatus represents the overall status of a workflow.
type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusPaused    WorkflowStatus = "paused"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
	StatusCancelled WorkflowStatus = "cancelled"
	StatusRollback  WorkflowStatus = "rolling_back"
)

// Engine manages one active workflow and publishes immutable snapshots.
type Engine struct {
	mu             sync.RWMutex
	checkpoints    CheckpointService
	executor       StepExecutor
	broker         *pubsub.Broker[Checkpoint]
	activeWorkflow *Checkpoint
	activeCancel   context.CancelFunc
	activeDone     chan struct{}
	workingDir     string
}

// NewEngine creates a workflow engine with personal checkpoint storage.
func NewEngine(workingDir string, selected ...string) *Engine {
	return &Engine{
		checkpoints: NewCheckpointService(workingDir, selected...),
		executor:    NewDefaultStepExecutor(),
		broker:      pubsub.NewBroker[Checkpoint](),
		workingDir:  workingDir,
	}
}

// Start saves and launches a new workflow, returning its initial snapshot.
func (e *Engine) Start(ctx context.Context, definition *Definition, contextData map[string]string) (*Checkpoint, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.activeWorkflow != nil {
		return nil, fmt.Errorf("workflow %s is already running", e.activeWorkflow.ID)
	}
	if definition == nil || len(definition.Steps) == 0 {
		return nil, fmt.Errorf("workflow requires at least one step")
	}
	steps := make([]Step, len(definition.Steps))
	seen := map[string]bool{}
	for i, step := range definition.Steps {
		if step == nil || step.ID == "" || seen[step.ID] {
			return nil, fmt.Errorf("workflow step IDs must be nonempty and unique")
		}
		seen[step.ID] = true
		steps[i] = *step
		steps[i].Status = StepStatusPending
		steps[i].StartedAt = nil
		steps[i].CompletedAt = nil
		steps[i].Error = ""
		steps[i].Output = ""
	}
	checkpoint := NewCheckpoint(definition.Name, definition.Name, steps)
	if contextData != nil {
		checkpoint.Context = contextData
	}
	checkpoint = cloneCheckpoint(checkpoint)
	checkpoint.Status = StatusRunning
	if err := e.checkpoints.Save(checkpoint); err != nil {
		return nil, fmt.Errorf("failed to save initial checkpoint: %w", err)
	}
	snapshot := cloneCheckpoint(checkpoint)
	e.launchLocked(checkpoint)
	return snapshot, nil
}

// launchLocked detaches the long-lived workflow from a single tool request.
// Cancel controls its lifetime explicitly through the stored cancellation.
func (e *Engine) launchLocked(checkpoint *Checkpoint) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	e.activeWorkflow = cloneCheckpoint(checkpoint)
	e.activeCancel = cancel
	e.activeDone = done
	go func() {
		defer func() {
			cancel()
			e.mu.Lock()
			if e.activeDone == done {
				e.activeWorkflow = nil
				e.activeCancel = nil
				e.activeDone = nil
			}
			close(done)
			e.mu.Unlock()
		}()
		e.executeWorkflow(ctx, checkpoint)
	}()
}

// record persists before publishing so consumers can immediately reload.
func (e *Engine) record(checkpoint *Checkpoint) bool {
	err := e.checkpoints.Save(checkpoint)
	if err != nil {
		checkpoint.Status = StatusFailed
		checkpoint.Error = fmt.Sprintf("failed to save checkpoint: %v", err)
		slog.Error("Workflow checkpoint failed", "error", err)
	}
	e.mu.Lock()
	if e.activeWorkflow != nil && e.activeWorkflow.ID == checkpoint.ID {
		if checkpoint.Status == StatusRunning || checkpoint.Status == StatusRollback {
			e.activeWorkflow = cloneCheckpoint(checkpoint)
		} else {
			e.activeWorkflow = nil
		}
	}
	e.mu.Unlock()
	e.broker.Publish(pubsub.CreatedEvent, *cloneCheckpoint(checkpoint))
	return err == nil
}

func (e *Engine) executeWorkflow(ctx context.Context, checkpoint *Checkpoint) {
	completed := make(map[string]bool)
	for _, step := range checkpoint.Steps {
		if step.Status == StepStatusCompleted {
			completed[step.ID] = true
		}
	}
	for {
		if ctx.Err() != nil {
			now := time.Now()
			checkpoint.Status = StatusCancelled
			checkpoint.CompletedAt = &now
			e.record(checkpoint)
			return
		}
		pending := -1
		unfinished := false
		for i := range checkpoint.Steps {
			step := &checkpoint.Steps[i]
			if step.Status == StepStatusCompleted || step.Status == StepStatusSkipped {
				continue
			}
			unfinished = true
			if step.CanStart(completed) {
				pending = i
				break
			}
		}
		if pending < 0 {
			if unfinished {
				checkpoint.Status = StatusFailed
				checkpoint.Error = "workflow blocked by unfinished or missing dependencies"
			} else {
				now := time.Now()
				checkpoint.Status = StatusCompleted
				checkpoint.CompletedAt = &now
			}
			e.record(checkpoint)
			return
		}
		checkpoint.CurrentStep = pending
		step := &checkpoint.Steps[pending]
		result, err := e.executor.Execute(ctx, step)
		if ctx.Err() != nil {
			now := time.Now()
			checkpoint.Status = StatusCancelled
			checkpoint.CompletedAt = &now
			e.record(checkpoint)
			return
		}
		if err == nil && result == nil {
			err = fmt.Errorf("step %s returned no execution result", step.ID)
		}
		if err != nil {
			step.Status = StepStatusFailed
			step.Error = err.Error()
			checkpoint.Status = StatusFailed
			checkpoint.Error = err.Error()
			e.rollbackWorkflow(ctx, checkpoint)
			return
		}
		step.Output = result.Output
		if result.NeedsApproval {
			step.Status = StepStatusAwaiting
			checkpoint.Status = StatusPaused
			e.record(checkpoint)
			return
		}
		if result.Success {
			step.Status = StepStatusCompleted
			completed[step.ID] = true
		} else if result.Skipped {
			step.Status = StepStatusSkipped
		} else {
			step.Status = StepStatusFailed
			step.Error = result.Error
			checkpoint.Status = StatusFailed
			checkpoint.Error = result.Error
			if checkpoint.Error == "" {
				checkpoint.Error = "step execution failed without an explanation"
			}
			e.rollbackWorkflow(ctx, checkpoint)
			return
		}
		checkpoint.CurrentStep = pending + 1
		if !e.record(checkpoint) {
			return
		}
	}
}

func (e *Engine) rollbackWorkflow(ctx context.Context, checkpoint *Checkpoint) {
	checkpoint.Status = StatusRollback
	if !e.record(checkpoint) {
		return
	}
	for i := len(checkpoint.Steps) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			break
		}
		step := &checkpoint.Steps[i]
		if step.Status == StepStatusCompleted && step.RollbackCmd != "" {
			if err := e.executor.Rollback(ctx, step); err != nil {
				step.Error = fmt.Sprintf("rollback failed: %v", err)
				checkpoint.Error += "; " + step.Error
				slog.Error("Rollback failed", "step", step.ID, "error", err)
			}
		}
	}
	checkpoint.Status = StatusFailed
	if ctx.Err() != nil {
		checkpoint.Status = StatusCancelled
	}
	e.record(checkpoint)
}

// Resume approves the current approval gate and resumes unfinished steps.
func (e *Engine) Resume(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.activeWorkflow != nil {
		return nil, fmt.Errorf("workflow %s is already running", e.activeWorkflow.ID)
	}
	checkpoint, err := e.checkpoints.Load(checkpointID)
	if err != nil {
		return nil, err
	}
	if checkpoint.Status != StatusPaused {
		return nil, fmt.Errorf("workflow is not paused (status: %s)", checkpoint.Status)
	}
	awaiting := false
	for i := range checkpoint.Steps {
		if checkpoint.Steps[i].Status == StepStatusAwaiting {
			now := time.Now()
			checkpoint.Steps[i].Status = StepStatusCompleted
			checkpoint.Steps[i].CompletedAt = &now
			checkpoint.CurrentStep = i + 1
			awaiting = true
			break
		}
	}
	if !awaiting {
		return nil, fmt.Errorf("paused workflow has no step awaiting approval")
	}
	checkpoint.Status = StatusRunning
	if err := e.checkpoints.Save(checkpoint); err != nil {
		return nil, err
	}
	snapshot := cloneCheckpoint(checkpoint)
	e.launchLocked(checkpoint)
	return snapshot, nil
}

// Cancel interrupts an active executor and waits for its durable result.
func (e *Engine) Cancel(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	e.mu.Lock()
	if e.activeWorkflow != nil && e.activeWorkflow.ID == checkpointID {
		cancel, done := e.activeCancel, e.activeDone
		cancel()
		e.mu.Unlock()
		select {
		case <-done:
			return e.checkpoints.Load(checkpointID)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer e.mu.Unlock()
	checkpoint, err := e.checkpoints.Load(checkpointID)
	if err != nil {
		return nil, err
	}
	if checkpoint.Status != StatusRunning && checkpoint.Status != StatusPaused {
		return nil, fmt.Errorf("workflow is not running or paused (status: %s)", checkpoint.Status)
	}
	now := time.Now()
	checkpoint.Status = StatusCancelled
	checkpoint.CompletedAt = &now
	if err := e.checkpoints.Save(checkpoint); err != nil {
		return nil, err
	}
	e.broker.Publish(pubsub.CreatedEvent, *cloneCheckpoint(checkpoint))
	return cloneCheckpoint(checkpoint), nil
}

// Approve accepts an approval gate or cancels a rejected workflow.
func (e *Engine) Approve(ctx context.Context, checkpointID string, approved bool) (*Checkpoint, error) {
	if approved {
		return e.Resume(ctx, checkpointID)
	}
	return e.Cancel(ctx, checkpointID)
}

// Status returns the latest durable workflow snapshot.
func (e *Engine) Status(checkpointID string) (*Checkpoint, error) {
	return e.checkpoints.Load(checkpointID)
}

// List returns saved workflows, optionally filtered by name.
func (e *Engine) List(workflowName string) ([]*Checkpoint, error) {
	return e.checkpoints.List(workflowName)
}

// GetActive returns an independent snapshot of the active workflow.
func (e *Engine) GetActive() *Checkpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneCheckpoint(e.activeWorkflow)
}

// Subscribe returns immutable workflow status snapshots.
func (e *Engine) Subscribe(ctx context.Context) <-chan pubsub.Event[Checkpoint] {
	return e.broker.Subscribe(ctx)
}
