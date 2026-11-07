package model

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func pressPaletteCommand(m *UI, name string) tea.Cmd {
	for _, r := range name {
		m.handleDialogMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func newReviewedNavigationUI() (*UI, *dialog.Overlay) {
	base, _ := newBuiltinTestUI(testConfigWithModel(1000))
	m := newTestUI()
	m.com.Workspace = base.com.Workspace
	m.session = base.session
	m.dialog = dialog.NewOverlay()
	return m, m.dialog
}

func TestPaletteArgumentCommandsLeaveEditorVisible(t *testing.T) {
	for _, name := range []string{"goal", "btw"} {
		t.Run(name, func(t *testing.T) {
			m, overlay := newReviewedNavigationUI()
			palette, err := dialog.NewCommands(m.com, "sess-1", true, false, false, nil, nil)
			require.NoError(t, err)
			overlay.OpenDialog(palette)
			pressPaletteCommand(m, name)
			require.Equal(t, "/"+name+" ", m.textarea.Value())
			require.False(t, overlay.HasDialogs(), "argument entry must not be hidden by an unfinishable Loading panel")
		})
	}
}

func TestPalettePreconditionWarningDoesNotLeaveLoading(t *testing.T) {
	m, _ := newReviewedNavigationUI()
	m.session = nil
	palette, err := dialog.NewCommands(m.com, "", false, false, false, nil, nil)
	require.NoError(t, err)
	m.dialog.OpenDialog(palette)
	cmd := pressPaletteCommand(m, "file-history")
	for _, msg := range executeInitializationCommands(cmd) {
		m.Update(msg)
	}
	if info, ok := m.dialog.DialogLast().(*dialog.Info); ok {
		require.NotEqual(t, "Loading", info.Title(), "the immediate warning must not leave an asynchronous placeholder")
	}
}

type reviewedTaskWorkspace struct{ workspace.Workspace }

func (*reviewedTaskWorkspace) Tasks(context.Context) ([]agent.RuntimeTask, error) { return nil, nil }

func TestPaletteAsyncResultRetainsParentForBack(t *testing.T) {
	m, _ := newReviewedNavigationUI()
	m.com.Workspace = &reviewedTaskWorkspace{m.com.Workspace}
	warmCaches(m, false)
	palette, err := dialog.NewCommands(m.com, "s1", true, false, false, nil, nil)
	require.NoError(t, err)
	m.dialog.OpenDialog(palette)
	cmd := pressPaletteCommand(m, "tasks")
	require.False(t, m.dialog.HasDialogs(), "do not invent a loading view for a deferred result")
	for _, msg := range executeInitializationCommands(cmd) {
		m.Update(msg)
	}
	require.Equal(t, dialog.InfoID, m.dialog.FrontID())
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	require.Same(t, palette, m.dialog.DialogLast())
}
