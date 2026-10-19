package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
)

type TaskStatusParams struct {
	TaskID string `json:"task_id" description:"The unique ID of the asynchronous task to check"`
}

func NewMCPTaskStatusTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"mcp_task_status",
		"Check the status and retrieve results of an asynchronous sandboxed task.",
		func(ctx context.Context, params TaskStatusParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.TaskID == "" {
				return fantasy.NewTextErrorResponse("missing task_id"), nil
			}

			task, ok := mcp.GetTask(params.TaskID)
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("task %s not found", params.TaskID)), nil
			}

			response := fmt.Sprintf("Task ID: %s\nStatus: %s\nCreated: %s\n", task.ID, task.Status, task.CreatedAt.Format("15:04:05"))
			if task.Status == mcp.TaskStatusCompleted && task.Result != nil {
				return fantasy.NewTextResponse(response + "\nResult:\n" + task.Result.Content), nil
			}
			if task.Status == mcp.TaskStatusFailed {
				return fantasy.NewTextErrorResponse(response + "\nError: " + task.Error.Error()), nil
			}

			return fantasy.NewTextResponse(response + "\nTask is still in progress. Please check back later."), nil
		},
	)
}
