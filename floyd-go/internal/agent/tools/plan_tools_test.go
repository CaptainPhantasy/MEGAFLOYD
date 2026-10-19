package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/stretchr/testify/require"
)

func planToolContext(sessionID string) context.Context {
	return context.WithValue(context.Background(), SessionIDContextKey, sessionID)
}

func TestEnterPlanModeTool(t *testing.T) {
	t.Parallel()

	plans := plan.NewService(t.TempDir())
	tool := NewEnterPlanModeTool(plans)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{ID: "1", Name: EnterPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no active session")

	resp, err = tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "2", Name: EnterPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "Plan mode is now active")
	require.Contains(t, resp.Content, "exit_plan_mode")

	status, err := plans.Status("session-1")
	require.NoError(t, err)
	require.True(t, status.Active)
	require.Contains(t, resp.Content, status.Path)

	resp, err = tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "3", Name: EnterPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "already active")
}

func TestExitPlanModeToolRejectsEmptyPlan(t *testing.T) {
	t.Parallel()

	plans := plan.NewService(t.TempDir())
	perms := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewExitPlanModeTool(plans, perms)

	resp, err := tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "1", Name: ExitPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "can only be called while plan mode is active")

	entered, err := plans.Enter("session-1")
	require.NoError(t, err)

	resp, err = tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "2", Name: ExitPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "No plan file found")
	require.Contains(t, resp.Content, entered.Path)
}

func TestExitPlanModeToolRecordsRevisionAndApproves(t *testing.T) {
	t.Parallel()

	plans := plan.NewService(t.TempDir())
	perms := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewExitPlanModeTool(plans, perms)

	entered, err := plans.Enter("session-1")
	require.NoError(t, err)
	planText := "# Plan\n\n1. Implement it."
	require.NoError(t, os.WriteFile(entered.Path, []byte(planText), 0o644))

	resp, err := tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "1", Name: ExitPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "## Approved Plan:")
	require.Contains(t, resp.Content, planText)
	require.Contains(t, resp.Content, "Plan saved to: "+entered.Path)

	status, err := plans.Status("session-1")
	require.NoError(t, err)
	require.False(t, status.Active)

	revision, err := os.ReadFile(filepath.Join(plans.RevisionsDir("session-1", entered.ID), "v1.md"))
	require.NoError(t, err)
	require.Equal(t, planText, string(revision))

	resp, err = tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "2", Name: ExitPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "can only be called while plan mode is active")
}

func TestExitPlanModeToolYoloWording(t *testing.T) {
	t.Parallel()

	plans := plan.NewService(t.TempDir())
	perms := permission.NewPermissionService(t.TempDir(), true, nil)
	tool := NewExitPlanModeTool(plans, perms)

	entered, err := plans.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Plan"), 0o644))

	resp, err := tool.Run(planToolContext("session-1"), fantasy.ToolCall{ID: "1", Name: ExitPlanModeToolName, Input: `{}`})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "auto-approved, not user-reviewed")
	require.Contains(t, resp.Content, "the user has NOT explicitly approved it")
}
