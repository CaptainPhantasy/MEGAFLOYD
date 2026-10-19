package model

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/stretchr/testify/require"
)

func TestRefreshPlanStateFollowsDurableState(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)
	svc := plan.NewService(cfg.Options.DataDirectory)

	cmd := ui.refreshPlanState("sess-1")
	require.NotNil(t, cmd)
	ui.Update(cmd())
	require.False(t, ui.status.planActive, "no active plan must hide the pill")

	_, err := svc.Enter("sess-1")
	require.NoError(t, err)

	cmd = ui.refreshPlanState("sess-1")
	require.NotNil(t, cmd)
	ui.Update(cmd())
	require.True(t, ui.status.planActive, "an active plan read from disk must show the pill")

	require.NoError(t, svc.Cancel("sess-1"))
	cmd = ui.refreshPlanState("sess-1")
	require.NotNil(t, cmd)
	ui.Update(cmd())
	require.False(t, ui.status.planActive, "cancelling off-thread must hide the pill")
}

func TestPlanRefreshTickGenerationIsGuarded(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)
	ui.planRefreshGeneration = 7

	require.Nil(t, ui.handlePlanRefreshTick(planRefreshTickMsg{SessionID: "sess-1", Generation: 6}),
		"a superseded tick must not read the plan file")
	require.NotNil(t, ui.handlePlanRefreshTick(planRefreshTickMsg{SessionID: "sess-1", Generation: 7}),
		"the newest tick must read the plan file")
	require.Nil(t, ui.handlePlanRefreshTick(planRefreshTickMsg{SessionID: "sess-2", Generation: 7}),
		"a tick for another session must be ignored")
}

func TestRequestPlanRefreshRequiresSession(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)
	ui.session = nil

	require.Nil(t, ui.requestPlanRefresh(), "no session means no plan state to read")
}

func TestToolMessageDrivesPlanPillRefresh(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)
	svc := plan.NewService(cfg.Options.DataDirectory)

	// Plain text traffic must not schedule a plan read.
	require.Nil(t, ui.planRefreshForMessage(message.Message{ID: "m1", SessionID: "sess-1"}))

	_, err := svc.Enter("sess-1")
	require.NoError(t, err)

	enter := message.Message{
		ID:        "m2",
		SessionID: "sess-1",
		Parts:     []message.ContentPart{message.ToolCall{ID: "call-1", Name: "enter_plan_mode"}},
	}
	cmd := ui.planRefreshForMessage(enter)
	require.NotNil(t, cmd, "tool traffic must schedule a plan read")

	tick, ok := cmd().(planRefreshTickMsg)
	require.True(t, ok)
	require.Equal(t, "sess-1", tick.SessionID)

	read := ui.handlePlanRefreshTick(tick)
	require.NotNil(t, read)
	ui.Update(read())
	require.True(t, ui.status.planActive, "agent-entered plan mode must show the pill")

	require.NoError(t, svc.Cancel("sess-1"))
	exit := message.Message{
		ID:        "m2",
		SessionID: "sess-1",
		Parts:     []message.ContentPart{message.ToolResult{ToolCallID: "call-1", Name: "exit_plan_mode"}},
	}
	cmd = ui.planRefreshForMessage(exit)
	require.NotNil(t, cmd)

	tick, ok = cmd().(planRefreshTickMsg)
	require.True(t, ok)
	read = ui.handlePlanRefreshTick(tick)
	require.NotNil(t, read)
	ui.Update(read())
	require.False(t, ui.status.planActive, "agent-exited plan mode must hide the pill")
}
