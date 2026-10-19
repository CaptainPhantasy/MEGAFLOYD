package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
)

const ExitPlanModeToolName = "exit_plan_mode"

//go:embed exit_plan_mode.md
var exitPlanModeDescription string

// ExitPlanModeParams is empty: the plan is read from the plan file.
type ExitPlanModeParams struct{}

// NewExitPlanModeTool creates the tool that records a plan revision,
// leaves plan mode, and returns the plan for the conversation.
func NewExitPlanModeTool(plans *plan.Service, permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExitPlanModeToolName,
		exitPlanModeDescription,
		func(ctx context.Context, _ ExitPlanModeParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("no active session"), nil
			}

			status, err := plans.Status(sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to read plan file: %v", err)), nil
			}
			if !status.Active {
				return fantasy.NewTextErrorResponse(
					"exit_plan_mode can only be called while plan mode is active. Use enter_plan_mode (or /plan) first.",
				), nil
			}
			if strings.TrimSpace(status.Content) == "" {
				if status.Path == "" {
					return fantasy.NewTextErrorResponse(
						"No plan file found. Write the plan to the current plan file first, then call exit_plan_mode.",
					), nil
				}
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"No plan file found. Write your plan to %s first, then call exit_plan_mode.",
					status.Path,
				)), nil
			}

			if _, err := plans.RecordRevision(sessionID); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to record plan revision: %v", err)), nil
			}
			if err := plans.Exit(sessionID); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to exit plan mode: %v", err)), nil
			}

			if permissions != nil && permissions.SkipRequests() {
				return fantasy.NewTextResponse(formatAutoApprovedPlan(status.Content, status.Path)), nil
			}
			return fantasy.NewTextResponse(formatApprovedPlan(status.Content, status.Path)), nil
		},
	)
}

func formatAutoApprovedPlan(planContent, planPath string) string {
	savedTo := ""
	if planPath != "" {
		savedTo = "Plan saved to: " + planPath + "\n\n"
	}
	return "Plan mode deactivated. All tools are now available.\n" +
		"Note: this plan was auto-approved without user review — the user has NOT explicitly approved it. " +
		"Follow the user's original instructions on whether to proceed with execution; if they asked you to stop, " +
		"wait, or only summarize after planning, do not start executing.\n" +
		savedTo +
		"## Plan (auto-approved, not user-reviewed):\n" + planContent
}

func formatApprovedPlan(planContent, planPath string) string {
	savedTo := ""
	if planPath != "" {
		savedTo = "Plan saved to: " + planPath + "\n\n"
	}
	return "Plan mode deactivated. All tools are now available.\n" +
		savedTo +
		"## Approved Plan:\n" + planContent
}
