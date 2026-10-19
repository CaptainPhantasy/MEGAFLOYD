package mcp

import (
	"time"

	"github.com/legacy-ai/floyd/internal/csync"
)

// TaskStatus represents the status of an async MCP task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// Task represents a long-running sandboxed job (SEP-1686).
type Task struct {
	ID        string
	Server    string
	Tool      string
	Status    TaskStatus
	Result    *ToolResult
	Error     error
	CreatedAt time.Time
}

var activeTasks = csync.NewMap[string, *Task]()

// GetTask returns the current status of an async task.
func GetTask(id string) (*Task, bool) {
	return activeTasks.Get(id)
}

// SetTask records or updates an async task in the registry.
func SetTask(task *Task) {
	activeTasks.Set(task.ID, task)
}
