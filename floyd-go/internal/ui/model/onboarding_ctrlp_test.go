package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

// TestCtrlPOpensCommandsDuringOnboarding pins the documented behavior: the
// onboarding screen tells the user ctrl+p works. Onboarding opens the model
// picker as a dialog, so the palette key must be handled before dialog
// routing or it is swallowed.
func TestCtrlPOpensCommandsDuringOnboarding(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	ui.keyMap = DefaultKeyMap()
	ui.state = uiOnboarding

	// Reproduce the real onboarding screen: the model picker is a dialog.
	ui.openModelsDialog()
	require.True(t, overlay.ContainsDialog(dialog.ModelsID), "onboarding must open the models dialog")

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})

	require.True(t, overlay.ContainsDialog(dialog.CommandsID), "ctrl+p must open the commands dialog over the onboarding picker")
}

// TestEscapeClosesPaletteOverOnboardingPicker pins the second half of the
// documented behavior: esc cancels the palette even while the onboarding
// model picker sits beneath it, and the picker itself still refuses to close.
func TestEscapeClosesPaletteOverOnboardingPicker(t *testing.T) {
	t.Parallel()

	ui, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	ui.keyMap = DefaultKeyMap()
	ui.state = uiOnboarding
	ui.openModelsDialog()
	require.True(t, overlay.ContainsDialog(dialog.ModelsID))

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	require.True(t, overlay.ContainsDialog(dialog.CommandsID))

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.False(t, overlay.ContainsDialog(dialog.CommandsID), "esc must close the palette")
	require.True(t, overlay.ContainsDialog(dialog.ModelsID), "the onboarding picker must stay open")

	ui.handleKeyPressMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.True(t, overlay.ContainsDialog(dialog.ModelsID), "esc must not close the onboarding picker")
}
