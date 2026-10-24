package tools

import (
	"charm.land/fantasy"
	"context"
	"encoding/json"
	"github.com/legacy-ai/floyd/internal/goal"
	"github.com/legacy-ai/floyd/internal/session"
)

const GoalToolName = "goal"

type GoalParams struct {
	Action string `json:"action" description:"status, complete, or blocked"`
	Reason string `json:"reason,omitempty" description:"Evidence of completion or the input needed to continue"`
}

func NewGoalTool(sessions session.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(GoalToolName, "Inspect the user's active goal, mark it complete only when achieved, or blocked when input is required. This tool cannot create goals or increase budgets.", func(ctx context.Context, in GoalParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		id := GetSessionFromContext(ctx)
		var value *goal.State
		var err error
		switch in.Action {
		case "status":
			value, err = goal.Get(ctx, sessions, id)
		case "complete", "blocked":
			value, err = goal.Configure(ctx, sessions, id, goal.Input{Control: in.Action, Reason: in.Reason})
		default:
			return fantasy.NewTextErrorResponse("action must be status, complete, or blocked"), nil
		}
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		b, err := json.Marshal(value)
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.NewTextResponse(string(b)), nil
	})
}
