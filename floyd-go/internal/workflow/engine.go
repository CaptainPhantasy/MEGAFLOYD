package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"
)

// WorkflowStatus represents the overall status of a workflow
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

// Engine manages workflow execution
type Engine struct {
	mu             sync.RWMutex
	checkpoints    CheckpointService
	executor       StepExecutor
	broker         *pubsub.Broker[Checkpoint]
	activeWorkflow *Checkpoint
	workingDir     string
}

// NewEngine creates a new workflow engine
func NewEngine(workingDir string) *Engine {
	return &Engine{
		checkpoints: NewCheckpointService(workingDir),
		executor:    NewDefaultStepExecutor(),
		broker:      pubsub.NewBroker[Checkpoint](),
		workingDir:  workingDir,
	}
}

// Start begins a new workflow execution
func (e *Engine) Start(ctx context.Context, definition *Definition, contextData map[string]string) (*Checkpoint, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeWorkflow != nil && e.activeWorkflow.Status == StatusRunning {
		return nil, fmt.Errorf("workflow %s is already running", e.activeWorkflow.ID)
	}

	steps := make([]Step, len(definition.Steps))
	for i, stepDef := range definition.Steps {
		steps[i] = *stepDef
		steps[i].Status = StepStatusPending
	}

	checkpoint := NewCheckpoint(definition.Name, definition.Name, steps)
	if contextData != nil {
		checkpoint.Context = contextData
	}

	if err := e.checkpoints.Save(checkpoint); err != nil {
		return nil, fmt.Errorf("failed to save initial checkpoint: %w", err)
	}

	e.activeWorkflow = checkpoint
	checkpoint.Status = StatusRunning

	go e.executeWorkflow(context.Background(), checkpoint)

	return checkpoint, nil
}

// executeWorkflow runs the workflow steps
func (e *Engine) executeWorkflow(ctx context.Context, checkpoint *Checkpoint) {
	defer func() {
		e.mu.Lock()
		if e.activeWorkflow != nil && e.activeWorkflow.ID == checkpoint.ID {
			e.activeWorkflow = nil
		}
		e.mu.Unlock()
	}()

	completedSteps := make(map[string]bool)

	for checkpoint.CurrentStep < len(checkpoint.Steps) {
		step := &checkpoint.Steps[checkpoint.CurrentStep]

		if !step.CanStart(completedSteps) {
			found := false
			for i := range checkpoint.Steps {
				if checkpoint.Steps[i].CanStart(completedSteps) &&
					checkpoint.Steps[i].Status == StepStatusPending {
					checkpoint.CurrentStep = i
					found = true
					break
				}
			}
			if !found {
				slog.Warn("Workflow blocked", "workflow_id", checkpoint.ID)
				break
			}
			continue
		}

		result, err := e.executor.Execute(ctx, step)
		if err != nil {
			step.Status = StepStatusFailed
			step.Error = err.Error()
			checkpoint.Status = StatusFailed
			checkpoint.Error = err.Error()
			e.checkpoints.Save(checkpoint)
			e.broker.Publish(pubsub.CreatedEvent, *checkpoint)

			e.rollbackWorkflow(ctx, checkpoint)
			return
		}

		if result.NeedsApproval {
			checkpoint.Status = StatusPaused
			e.checkpoints.Save(checkpoint)
			e.broker.Publish(pubsub.CreatedEvent, *checkpoint)
			return
		}

		if result.Success {
			step.Status = StepStatusCompleted
			completedSteps[step.ID] = true
		} else if !result.Skipped {
			step.Status = StepStatusFailed
			step.Error = result.Error
			checkpoint.Status = StatusFailed
			checkpoint.Error = result.Error
			e.checkpoints.Save(checkpoint)
			e.broker.Publish(pubsub.CreatedEvent, *checkpoint)

			e.rollbackWorkflow(ctx, checkpoint)
			return
		}

		checkpoint.CurrentStep++
		e.checkpoints.Save(checkpoint)
		e.broker.Publish(pubsub.CreatedEvent, *checkpoint)
	}

	now := time.Now()
	checkpoint.Status = StatusCompleted
	checkpoint.CompletedAt = &now
	e.checkpoints.Save(checkpoint)
	e.broker.Publish(pubsub.CreatedEvent, *checkpoint)
}

// rollbackWorkflow attempts to rollback completed steps
func (e *Engine) rollbackWorkflow(ctx context.Context, checkpoint *Checkpoint) {
	checkpoint.Status = StatusRollback
	e.checkpoints.Save(checkpoint)
	e.broker.Publish(pubsub.CreatedEvent, *checkpoint)

	for i := len(checkpoint.Steps) - 1; i >= 0; i-- {
		step := &checkpoint.Steps[i]
		if step.Status == StepStatusCompleted && step.RollbackCmd != "" {
			if err := e.executor.Rollback(ctx, step); err != nil {
				slog.Error("Rollback failed", "step", step.ID, "error", err)
			}
		}
	}

	checkpoint.Status = StatusFailed
	e.checkpoints.Save(checkpoint)
	e.broker.Publish(pubsub.CreatedEvent, *checkpoint)
}

// Resume continues a paused workflow
func (e *Engine) Resume(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	checkpoint, err := e.checkpoints.Load(checkpointID)
	if err != nil {
		return nil, err
	}

	if checkpoint.Status != StatusPaused {
		return nil, fmt.Errorf("workflow is not paused (status: %s)", checkpoint.Status)
	}

	for i := range checkpoint.Steps {
		if checkpoint.Steps[i].Status == StepStatusAwaiting {
			now := time.Now()
			checkpoint.Steps[i].Status = StepStatusCompleted
			checkpoint.Steps[i].CompletedAt = &now
			checkpoint.CurrentStep = i + 1
			break
		}
	}

	checkpoint.Status = StatusRunning
	e.checkpoints.Save(checkpoint)
	e.activeWorkflow = checkpoint

	go e.executeWorkflow(context.Background(), checkpoint)

	return checkpoint, nil
}

// Cancel stops a running workflow
func (e *Engine) Cancel(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	e.mu.Lock()
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
	e.checkpoints.Save(checkpoint)
	e.broker.Publish(pubsub.CreatedEvent, *checkpoint)

	if e.activeWorkflow != nil && e.activeWorkflow.ID == checkpointID {
		e.activeWorkflow = nil
	}

	return checkpoint, nil
}

// Approve approves a step that is awaiting user input
func (e *Engine) Approve(ctx context.Context, checkpointID string, approved bool) (*Checkpoint, error) {
	if approved {
		return e.Resume(ctx, checkpointID)
	}
	return e.Cancel(ctx, checkpointID)
}

// Status returns the current status of a workflow
func (e *Engine) Status(checkpointID string) (*Checkpoint, error) {
	return e.checkpoints.Load(checkpointID)
}

// List returns all workflows, optionally filtered
func (e *Engine) List(workflowName string) ([]*Checkpoint, error) {
	return e.checkpoints.List(workflowName)
}

// GetActive returns the currently running workflow, if any
func (e *Engine) GetActive() *Checkpoint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.activeWorkflow
}

// Subscribe returns a channel for workflow status updates
func (e *Engine) Subscribe(ctx context.Context) <-chan pubsub.Event[Checkpoint] {
	return e.broker.Subscribe(ctx)
}
