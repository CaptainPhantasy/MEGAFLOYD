package model

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/experiments"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

type suggestionWorkspace struct {
	workspace.Workspace
	cfg      *config.Config
	calls    int
	canceled bool
}

func (w *suggestionWorkspace) Config() *config.Config { return w.cfg }
func (w *suggestionWorkspace) SuggestPrompt(ctx context.Context, sessionID, prefix string) (string, error) {
	w.calls++
	if err := ctx.Err(); err != nil {
		w.canceled = true
		return "", err
	}
	return "Add the regression test", nil
}

func TestCanonicalGhostSuggestionUsesSharedWorkspace(t *testing.T) {
	cfg := &config.Config{Experimental: map[string]bool{experiments.GhostText: true}}
	ws := &suggestionWorkspace{cfg: cfg}
	ui := &UI{com: &common.Common{Workspace: ws}, session: &session.Session{ID: "canonical"}, textarea: textarea.New(), focus: uiFocusEditor}
	ui.textarea.Focus()
	msg := message.Message{ID: "completed", SessionID: "canonical", Role: message.Assistant}
	require.Nil(t, ui.requestAISuggestion(msg))
	msg.AddFinish(message.FinishReasonEndTurn, "", "")
	cmd := ui.requestAISuggestion(msg)
	require.NotNil(t, cmd)
	require.Zero(t, ws.calls, "Update schedules I/O without running it")
	require.Nil(t, ui.requestAISuggestion(msg), "duplicate finish events do not add paid calls")
	result := cmd().(aiSuggestionMsg)
	require.Equal(t, 1, ws.calls)
	ui.applyAISuggestion(result)
	require.Equal(t, "Add the regression test", ui.commandSuggestion)
	ui.textarea.SetValue("Add")
	ui.updateCommandSuggestion()
	require.Equal(t, 1, ws.calls, "typing only filters the existing suggestion")
	require.Equal(t, "Add the regression test", ui.commandSuggestion)

	msg.ID = "next"
	cmd = ui.requestAISuggestion(msg)
	require.NotNil(t, cmd)
	ui.cancelAISuggestion()
	_ = cmd()
	require.True(t, ws.canceled)

	cfg.Experimental[experiments.GhostText] = false
	msg.ID = "disabled"
	require.Nil(t, ui.requestAISuggestion(msg))
	ui.updateCommandSuggestion()
	require.Empty(t, ui.commandSuggestion)
}
