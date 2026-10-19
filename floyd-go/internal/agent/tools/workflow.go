package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/workflow"
)

//go:embed workflow.md
var workflowDescription []byte

const WorkflowToolName = "workflow"

type WorkflowParams struct {
	Action       string            `json:"action" description:"Action to perform: start, status, resume, cancel, approve, list, definitions"`
	WorkflowName string            `json:"workflow_name,omitempty" description:"Name of the workflow definition to start"`
	CheckpointID string            `json:"checkpoint_id,omitempty" description:"ID of a checkpoint to resume, cancel, or approve"`
	Approved     *bool             `json:"approved,omitempty" description:"For approve action: true to approve, false to reject"`
	Context      map[string]string `json:"context,omitempty" description:"Optional context data for the workflow"`
}

type WorkflowResponseMetadata struct {
	Action       string                  `json:"action"`
	CheckpointID string                  `json:"checkpoint_id,omitempty"`
	Status       workflow.WorkflowStatus `json:"status,omitempty"`
}

func NewWorkflowTool(engine *workflow.Engine) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WorkflowToolName,
		string(workflowDescription),
		func(ctx context.Context, params WorkflowParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			switch params.Action {
			case "definitions", "list_definitions":
				return handleListDefinitions(engine, params)
			case "start":
				return handleStart(ctx, engine, params)
			case "status":
				return handleStatus(engine, params)
			case "resume":
				return handleResume(ctx, engine, params)
			case "cancel":
				return handleCancel(ctx, engine, params)
			case "approve":
				return handleApprove(ctx, engine, params)
			case "list":
				return handleList(engine, params)
			default:
				return fantasy.ToolResponse{}, fmt.Errorf("unknown action: %s", params.Action)
			}
		})
}

func handleListDefinitions(engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	definitions := workflow.ListDefinitions()
	predefined := workflow.GetPredefinedWorkflows()

	response := "# Available Workflow Definitions\n\n"
	for _, name := range definitions {
		def := predefined[name]
		response += fmt.Sprintf("## %s\n%s\n\n**Steps:**\n", name, def.Description)
		for _, step := range def.Steps {
			response += fmt.Sprintf("- %s (%s): %s\n", step.Name, step.Type, step.Description)
		}
		response += "\n"
	}

	metadata := WorkflowResponseMetadata{Action: "definitions"}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleStart(ctx context.Context, engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	if params.WorkflowName == "" {
		return fantasy.ToolResponse{}, fmt.Errorf("workflow_name is required for start action")
	}

	def, ok := workflow.GetDefinition(params.WorkflowName)
	if !ok {
		return fantasy.ToolResponse{}, fmt.Errorf("unknown workflow: %s. Use 'definitions' action to see available workflows", params.WorkflowName)
	}

	checkpoint, err := engine.Start(ctx, def, params.Context)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to start workflow: %w", err)
	}

	response := fmt.Sprintf("# Workflow Started\n\n**Workflow:** %s\n**Checkpoint ID:** %s\n**Status:** %s\n\n",
		checkpoint.WorkflowName, checkpoint.ID, checkpoint.Status)

	response += "## Steps\n"
	for i, step := range checkpoint.Steps {
		status := string(step.Status)
		if i == checkpoint.CurrentStep {
			status = "**" + status + "** (current)"
		}
		response += fmt.Sprintf("%d. %s - %s (%s)\n", i+1, step.Name, step.Type, status)
	}

	metadata := WorkflowResponseMetadata{
		Action:       "start",
		CheckpointID: checkpoint.ID,
		Status:       checkpoint.Status,
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleStatus(engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	if params.CheckpointID == "" {
		active := engine.GetActive()
		if active == nil {
			return fantasy.NewTextResponse("No active workflow. Use 'list' to see all workflows."), nil
		}
		return formatWorkflowStatus(active)
	}

	checkpoint, err := engine.Status(params.CheckpointID)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to get workflow status: %w", err)
	}

	return formatWorkflowStatus(checkpoint)
}

func formatWorkflowStatus(checkpoint *workflow.Checkpoint) (fantasy.ToolResponse, error) {
	response := fmt.Sprintf("# Workflow Status\n\n**Workflow:** %s\n**Checkpoint ID:** %s\n**Status:** %s\n\n",
		checkpoint.WorkflowName, checkpoint.ID, checkpoint.Status)

	if checkpoint.Error != "" {
		response += fmt.Sprintf("**Error:** %s\n\n", checkpoint.Error)
	}

	response += "## Steps\n"
	for i, step := range checkpoint.Steps {
		marker := " "
		if step.Status == workflow.StepStatusRunning {
			marker = "▶"
		} else if step.Status == workflow.StepStatusCompleted {
			marker = "✓"
		} else if step.Status == workflow.StepStatusFailed {
			marker = "✗"
		} else if step.Status == workflow.StepStatusAwaiting {
			marker = "⏸"
		}

		response += fmt.Sprintf("%s %d. %s (%s) - %s\n", marker, i+1, step.Name, step.Type, step.Status)
		if step.Error != "" {
			response += fmt.Sprintf("   Error: %s\n", step.Error)
		}
	}

	metadata := WorkflowResponseMetadata{
		Action:       "status",
		CheckpointID: checkpoint.ID,
		Status:       checkpoint.Status,
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleResume(ctx context.Context, engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	if params.CheckpointID == "" {
		return fantasy.ToolResponse{}, fmt.Errorf("checkpoint_id is required for resume action")
	}

	checkpoint, err := engine.Resume(ctx, params.CheckpointID)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to resume workflow: %w", err)
	}

	response := fmt.Sprintf("# Workflow Resumed\n\n**Workflow:** %s\n**Checkpoint ID:** %s\n**Status:** %s\n",
		checkpoint.WorkflowName, checkpoint.ID, checkpoint.Status)

	metadata := WorkflowResponseMetadata{
		Action:       "resume",
		CheckpointID: checkpoint.ID,
		Status:       checkpoint.Status,
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleCancel(ctx context.Context, engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	if params.CheckpointID == "" {
		return fantasy.ToolResponse{}, fmt.Errorf("checkpoint_id is required for cancel action")
	}

	checkpoint, err := engine.Cancel(ctx, params.CheckpointID)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to cancel workflow: %w", err)
	}

	response := fmt.Sprintf("# Workflow Cancelled\n\n**Workflow:** %s\n**Checkpoint ID:** %s\n**Status:** %s\n",
		checkpoint.WorkflowName, checkpoint.ID, checkpoint.Status)

	metadata := WorkflowResponseMetadata{
		Action:       "cancel",
		CheckpointID: checkpoint.ID,
		Status:       checkpoint.Status,
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleApprove(ctx context.Context, engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	if params.CheckpointID == "" {
		return fantasy.ToolResponse{}, fmt.Errorf("checkpoint_id is required for approve action")
	}
	if params.Approved == nil {
		return fantasy.ToolResponse{}, fmt.Errorf("approved parameter is required for approve action")
	}

	checkpoint, err := engine.Approve(ctx, params.CheckpointID, *params.Approved)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to approve workflow: %w", err)
	}

	action := "approved"
	if !*params.Approved {
		action = "rejected"
	}

	response := fmt.Sprintf("# Workflow %s\n\n**Workflow:** %s\n**Checkpoint ID:** %s\n**Status:** %s\n",
		action, checkpoint.WorkflowName, checkpoint.ID, checkpoint.Status)

	metadata := WorkflowResponseMetadata{
		Action:       "approve",
		CheckpointID: checkpoint.ID,
		Status:       checkpoint.Status,
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}

func handleList(engine *workflow.Engine, params WorkflowParams) (fantasy.ToolResponse, error) {
	checkpoints, err := engine.List(params.WorkflowName)
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("failed to list workflows: %w", err)
	}

	if len(checkpoints) == 0 {
		response := "No workflows found."
		if params.WorkflowName != "" {
			response = fmt.Sprintf("No workflows found for type: %s", params.WorkflowName)
		}
		return fantasy.NewTextResponse(response), nil
	}

	response := "# Workflows\n\n"
	for _, cp := range checkpoints {
		response += fmt.Sprintf("- **%s** (%s) - Status: %s - ID: `%s`\n",
			cp.WorkflowName, cp.CreatedAt.Format("2006-01-02 15:04"), cp.Status, cp.ID)
	}

	metadata := WorkflowResponseMetadata{Action: "list"}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(response), metadata), nil
}
