package tools

import (
	"context"
	_ "embed"
	"fmt"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/plan"
)

const EnterPlanModeToolName = "enter_plan_mode"

//go:embed enter_plan_mode.md
var enterPlanModeDescription string

// EnterPlanModeParams is empty: entering plan mode takes no arguments.
type EnterPlanModeParams struct{}

// NewEnterPlanModeTool creates the tool that activates plan mode for the
// current session.
func NewEnterPlanModeTool(plans *plan.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		EnterPlanModeToolName,
		enterPlanModeDescription,
		func(ctx context.Context, _ EnterPlanModeParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("no active session"), nil
			}

			status, err := plans.Status(sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to read plan status: %v", err)), nil
			}
			if status.Active {
				return fantasy.NewTextErrorResponse(
					"Plan mode is already active. Use exit_plan_mode when the plan is ready.",
				), nil
			}

			entered, err := plans.Enter(sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to enter plan mode: %v", err)), nil
			}
			return fantasy.NewTextResponse(enteredPlanModeMessage(entered.Path)), nil
		},
	)
}

func enteredPlanModeMessage(planPath string) string {
	if planPath == "" {
		return "Plan mode is now active, but no plan file path is available. Do NOT use write or edit; no plan file can be written."
	}
	return fmt.Sprintf(
		"Plan mode is now active. Your workflow:\n\n"+
			"Plan file: %s\n\n"+
			"1. Use read-only tools (Read, Grep, Glob) to investigate the codebase. Use Bash only when needed.\n"+
			"2. Design a concrete, step-by-step plan.\n"+
			"3. Write the plan to the plan file with Write or Edit.\n"+
			"4. When the plan is ready, call exit_plan_mode for user approval.\n\n"+
			"Do NOT edit files other than the plan file while plan mode is active.\n"+
			"Use Bash only when needed; Bash follows the normal permission mode and rules.",
		planPath,
	)
}
