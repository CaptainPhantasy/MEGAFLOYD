package model

import (
	"image"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

func newTerminalTestUI(t *testing.T) *UI {
	t.Helper()
	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = t.TempDir()
	ui, _ := newBuiltinTestUI(cfg)
	ui.com.Workspace = &builtinTestWorkspace{cfg: cfg, workDir: t.TempDir()}
	ui.layout.main = image.Rect(0, 0, 80, 20)

	ta := textarea.New()
	ta.SetStyles(ui.com.Styles.Editor.Textarea)
	ta.SetVirtualCursor(false)
	ta.Focus()
	ui.textarea = ta
	return ui
}

func TestToggleTerminalStartsAndHidesComponent(t *testing.T) {
	t.Parallel()

	ui := newTerminalTestUI(t)
	require.False(t, ui.terminalVisible)

	cmd := ui.toggleTerminal()
	require.NotNil(t, cmd, "the first toggle must start a shell")
	require.True(t, ui.terminalVisible)

	msg, ok := cmd().(terminalReadyMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)
	require.NotNil(t, msg.Component)
	require.NotNil(t, msg.Cmd, "the first PTY read must be scheduled")

	ui.terminal = msg.Component
	ui.terminal.Focus()
	require.True(t, ui.terminal.Active())

	// Hiding the pane keeps the shell alive.
	ui.toggleTerminal()
	require.False(t, ui.terminalVisible)
	require.True(t, ui.terminal.Active())

	ui.closeTerminal()
	require.Nil(t, ui.terminal)
	require.False(t, ui.terminalVisible)
}

func TestTerminalSizeIsClamped(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Options: &config.Options{}}
	ui, _ := newBuiltinTestUI(cfg)
	ui.layout.main = image.Rect(0, 0, 3, 2)

	cols, rows := ui.terminalSize()
	require.GreaterOrEqual(t, cols, 20)
	require.GreaterOrEqual(t, rows, 5)
}
