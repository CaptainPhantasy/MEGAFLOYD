package workflow

import (
	"context"
	"fmt"
	"time"
)

// StepType defines the type of workflow step
type StepType string

const (
	StepTypeAnalyze StepType = "analyze"
	StepTypePlan    StepType = "plan"
	StepTypeExecute StepType = "execute"
	StepTypeVerify  StepType = "verify"
	StepTypeApprove StepType = "approve"
)

// StepStatus represents the current status of a step
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusRollback  StepStatus = "rolled_back"
	StepStatusAwaiting  StepStatus = "awaiting_approval"
)

// Step represents a single step in a workflow
type Step struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        StepType               `json:"type"`
	Status      StepStatus             `json:"status"`
	Description string                 `json:"description"`
	Command     string                 `json:"command,omitempty"`
	Validation  string                 `json:"validation,omitempty"`
	Requires    []string               `json:"requires,omitempty"`
	RollbackCmd string                 `json:"rollback_cmd,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`

	// Execution state
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Output      string     `json:"output,omitempty"`
}

// CanStart checks if all required steps are completed
func (s *Step) CanStart(completedSteps map[string]bool) bool {
	for _, req := range s.Requires {
		if !completedSteps[req] {
			return false
		}
	}
	return true
}

// IsApprovalGate returns true if this step requires user approval
func (s *Step) IsApprovalGate() bool {
	return s.Type == StepTypeApprove
}

// StepExecutor defines the interface for executing workflow steps
type StepExecutor interface {
	Execute(ctx context.Context, step *Step) (*StepResult, error)
	Rollback(ctx context.Context, step *Step) error
	Validate(ctx context.Context, step *Step) (bool, error)
}

// StepResult contains the result of step execution
type StepResult struct {
	Output        string `json:"output"`
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
	Skipped       bool   `json:"skipped,omitempty"`
	NeedsApproval bool   `json:"needs_approval,omitempty"`
}

// DefaultStepExecutor provides basic step execution capabilities
type DefaultStepExecutor struct{}

func NewDefaultStepExecutor() *DefaultStepExecutor {
	return &DefaultStepExecutor{}
}

func (e *DefaultStepExecutor) Execute(ctx context.Context, step *Step) (*StepResult, error) {
	now := time.Now()
	step.StartedAt = &now
	step.Status = StepStatusRunning

	switch step.Type {
	case StepTypeApprove:
		step.Status = StepStatusAwaiting
		return &StepResult{
			Success:       false,
			NeedsApproval: true,
			Output:        "Waiting for user approval",
		}, nil

	case StepTypeAnalyze, StepTypePlan, StepTypeExecute, StepTypeVerify:
		completed := time.Now()
		step.CompletedAt = &completed
		step.Status = StepStatusCompleted
		return &StepResult{
			Success: true,
			Output:  fmt.Sprintf("Step %s completed", step.Name),
		}, nil

	default:
		step.Status = StepStatusFailed
		return &StepResult{
			Success: false,
			Error:   fmt.Sprintf("unknown step type: %s", step.Type),
		}, nil
	}
}

func (e *DefaultStepExecutor) Rollback(ctx context.Context, step *Step) error {
	if step.RollbackCmd == "" {
		return nil
	}

	now := time.Now()
	step.Status = StepStatusRollback
	step.CompletedAt = &now

	return nil
}

func (e *DefaultStepExecutor) Validate(ctx context.Context, step *Step) (bool, error) {
	if step.Validation == "" {
		return true, nil
	}

	return true, nil
}
