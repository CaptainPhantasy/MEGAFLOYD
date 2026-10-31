package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

func TestPaletteChildReturnsToSameFilteredParent(t *testing.T) {
	t.Parallel()
	m, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	palette, err := dialog.NewCommands(m.com, "sess-1", false, false, false, nil, nil)
	require.NoError(t, err)
	overlay.OpenDialog(palette)
	for _, r := range "theme" {
		m.handleDialogMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, dialog.ThemeID, overlay.FrontID())
	require.Same(t, palette, overlay.Dialog(dialog.CommandsID), "opening a child must retain the parent")
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	require.Same(t, palette, overlay.DialogLast())
	// The retained filter still selects Theme, so Enter returns to that child.
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, dialog.ThemeID, overlay.FrontID())
}
