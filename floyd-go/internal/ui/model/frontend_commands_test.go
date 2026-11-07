package model

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

type frontendCommandWorkspace struct {
	workspace.Workspace
	request proto.FrontendRequest
}

type librarySelectionDialog struct {
	dialog.Dialog
	action dialog.Action
	id     string
}

func (d *librarySelectionDialog) ID() string                      { return d.id }
func (d *librarySelectionDialog) HandleMsg(tea.Msg) dialog.Action { return d.action }

func (w *frontendCommandWorkspace) FrontendOperation(_ context.Context, r proto.FrontendRequest) (proto.FrontendResult, error) {
	w.request = r
	return proto.FrontendResult{Title: r.Kind, Body: "result", SessionID: "test-session"}, nil
}

func TestFrontendCommandArgumentsAndSessionGuidance(t *testing.T) {
	m, _ := newBuiltinTestUI(testConfigWithModel(1000))
	w := &frontendCommandWorkspace{Workspace: m.com.Workspace}
	m.com.Workspace = w
	result := m.libraryCommand("hooks", "explain PreToolUse bash")().(builtinResultMsg)
	require.NoError(t, result.Err)
	require.Equal(t, "PreToolUse", w.request.Argument)
	require.Equal(t, "bash", w.request.ToolName)
	result = m.libraryCommand("session", "export")().(builtinResultMsg)
	require.NotContains(t, result.Body, "Open the imported session")
	result = m.libraryCommand("import", "engine-test")().(builtinResultMsg)
	require.Contains(t, result.Body, "Open the imported session with /sessions: test-session")
	require.Equal(t, "engine-test", w.request.Argument)
	m.libraryCommand("telemetry", "ship http://127.0.0.1:4318")()
	require.Equal(t, "http://127.0.0.1:4318", w.request.Argument)
}

func TestLibrarySelectionRetainsDraftAndShowsEditor(t *testing.T) {
	for _, action := range []dialog.Action{
		dialog.ActionSelectAgent{AgentName: "Example", SystemPrompt: "Agent instructions"},
		dialog.ActionSelectSkill{SkillName: "Example", SkillContent: "Skill instructions"},
	} {
		base, _ := newBuiltinTestUI(testConfigWithModel(1000))
		m := newTestUI()
		m.com.Workspace = base.com.Workspace
		overlay := dialog.NewOverlay()
		m.dialog = overlay
		palette, err := dialog.NewCommands(m.com, "s1", true, false, false, nil, nil)
		require.NoError(t, err)
		overlay.OpenDialog(palette)
		m.textarea.SetValue("Existing draft")
		id := dialog.AgentLibraryID
		if _, ok := action.(dialog.ActionSelectSkill); ok {
			id = dialog.SkillsLibraryID
		}
		overlay.OpenDialog(&librarySelectionDialog{action: action, id: id})
		m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
		require.Contains(t, m.textarea.Value(), "Existing draft")
		require.Contains(t, m.textarea.Value(), "instructions")
		require.False(t, overlay.HasDialogs())
	}
}
