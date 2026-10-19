package model

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	"github.com/legacy-ai/floyd/internal/experiments"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/stretchr/testify/require"
)

func TestAddDirAddsPersistsAndLists(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	ws := ui.com.Workspace.(*builtinTestWorkspace)
	ws.workDir = t.TempDir()
	extra := t.TempDir()

	cmd := ui.runBuiltin("add-dir", extra, false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)
	require.Contains(t, msg.Info, extra)
	require.Equal(t, []string{extra}, ws.dirs)

	cmd = ui.runBuiltin("add-dir", "list", false)
	require.Nil(t, cmd)
	info, ok := overlay.Dialog(dialog.InfoID).(*dialog.Info)
	require.True(t, ok, "list must open the info panel")
	require.Contains(t, info.Body(), ws.workDir)
	require.Contains(t, info.Body(), extra)
}

func TestAddDirRejectsMissingPath(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))
	ws := ui.com.Workspace.(*builtinTestWorkspace)
	ws.workDir = t.TempDir()

	cmd := ui.runBuiltin("add-dir", "does-not-exist", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.Error(t, msg.Err)
	require.Empty(t, ws.dirs)
}

func TestThemeSelectionAppliesAndPersists(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))
	ws := ui.com.Workspace.(*builtinTestWorkspace)

	cmd := ui.runBuiltin("theme", "light", false)
	require.NotNil(t, cmd)
	require.Equal(t, "light", ui.themeName)
	require.NotEqual(t, styles.CharmtonePantera(), *ui.com.Styles, "light must differ from the dark palette")

	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)
	require.Equal(t, "light", ws.setFields["options.tui.theme"])
}

func TestThemeBareOpensPicker(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	cmd := ui.runBuiltin("theme", "", false)
	require.Nil(t, cmd)
	require.NotNil(t, overlay.Dialog(dialog.ThemeID), "bare /theme must open the picker")
}

func TestThemeUnknownNameIsRejected(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))
	ws := ui.com.Workspace.(*builtinTestWorkspace)

	cmd := ui.runBuiltin("theme", "solarized", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(util.InfoMsg)
	require.True(t, ok)
	require.Contains(t, msg.Msg, "unknown theme")
	require.Empty(t, ui.themeName)
	require.Nil(t, ws.setFields)
}

func TestExperimentsTogglePersists(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))
	ws := ui.com.Workspace.(*builtinTestWorkspace)

	cmd := ui.runBuiltin("experiments", "ghost_text off", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)
	require.Equal(t, false, ws.setFields["experimental.ghost_text"])
}

func TestExperimentsListShowsState(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	cfg.Experimental = map[string]bool{experiments.GhostText: false}
	ui, overlay := newBuiltinTestUI(cfg)

	cmd := ui.runBuiltin("experiments", "list", false)
	require.Nil(t, cmd)
	info, ok := overlay.Dialog(dialog.InfoID).(*dialog.Info)
	require.True(t, ok)
	require.Contains(t, info.Body(), experiments.GhostText)
	require.Contains(t, info.Body(), "[off]")
}

func TestExperimentsUnknownFlagIsRejected(t *testing.T) {
	t.Parallel()

	ui, _ := newBuiltinTestUI(testConfigWithModel(1000))
	cmd := ui.runBuiltin("experiments", "no_such_flag", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(util.InfoMsg)
	require.True(t, ok)
	require.Contains(t, msg.Msg, "Unknown experiment")
}

func TestGhostTextExperimentGatesSuggestions(t *testing.T) {
	t.Parallel()

	cfg := testConfigWithModel(1000)
	ui, _ := newBuiltinTestUI(cfg)
	ui.focus = uiFocusEditor
	ui.textarea = textarea.New()
	ui.textarea.Focus()
	ui.promptHistory.messages = []string{"deploy the release"}
	ui.textarea.SetValue("dep")

	ui.updateCommandSuggestion()
	require.Equal(t, "deploy the release", ui.commandSuggestion)

	// Toggling the flag through the command persists the override; the
	// stub applies it to the live config exactly as a config reload does.
	cmd := ui.runBuiltin("experiments", "ghost_text off", false)
	require.NotNil(t, cmd)
	msg, ok := cmd().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, msg.Err)

	ui.updateCommandSuggestion()
	require.Empty(t, ui.commandSuggestion)
}
