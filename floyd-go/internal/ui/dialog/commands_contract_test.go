package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/stretchr/testify/require"
)

func commandContractDialog(t *testing.T) *Commands {
	t.Helper()
	st := styles.CharmtonePantera()
	d, err := NewCommands(&common.Common{Workspace: &commandsTestWorkspace{cfg: &config.Config{}}, Styles: &st}, "", false, false, false, nil, nil)
	require.NoError(t, err)
	return d
}

func TestPaletteAdvertisedShortcutSelectsItsCommand(t *testing.T) {
	t.Parallel()
	d := commandContractDialog(t)
	d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	action := d.HandleMsg(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	run, ok := action.(ActionRunBuiltin)
	require.True(t, ok)
	require.Equal(t, "yolo", run.Name)
}

func TestPaletteOnlyAdvertisesAvailableCategoryNavigation(t *testing.T) {
	t.Parallel()
	d := commandContractDialog(t)
	for _, binding := range d.ShortHelp() {
		require.NotContains(t, binding.Keys(), "tab")
		require.NotContains(t, binding.Keys(), "shift+tab")
	}
	d.customCommands = []commands.CustomCommand{{Name: "sample"}}
	var forward, backward bool
	for _, binding := range d.ShortHelp() {
		for _, k := range binding.Keys() {
			forward = forward || k == "tab"
			backward = backward || k == "shift+tab"
		}
	}
	require.True(t, forward)
	require.True(t, backward)
	d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, UserCommands, d.selected)
	d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	require.Equal(t, SystemCommands, d.selected)
}

func TestPaletteRendersAdvertisedArguments(t *testing.T) {
	t.Parallel()
	st := styles.CharmtonePantera()
	for _, item := range builtinPaletteItems(&st) {
		spec, ok := commands.Builtin(item.title[1:])
		require.True(t, ok)
		if spec.ArgumentHint != "" {
			require.Contains(t, ansi.Strip(item.Render(120)), spec.ArgumentHint, spec.Name)
		}
	}
}

func TestPaletteEmptySelectionReportsAndRetainsInput(t *testing.T) {
	t.Parallel()
	d := commandContractDialog(t)
	for _, r := range "zzzznotacommand" {
		d.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	require.Empty(t, d.list.FilteredItems())
	action, ok := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter}).(ActionCmd)
	require.True(t, ok, "Enter must report why nothing can be selected")
	require.NotNil(t, action.Cmd)
	msg, ok := action.Cmd().(util.InfoMsg)
	require.True(t, ok)
	require.Equal(t, util.InfoTypeWarn, msg.Type)
	require.Contains(t, msg.Msg, "No matching command")
	require.Equal(t, "zzzznotacommand", d.input.Value())
}

func TestPaletteExactCommandBeatsArgumentHintMatch(t *testing.T) {
	c := commandContractDialog(t)
	for _, r := range "provider" {
		c.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	action, ok := c.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter}).(ActionRunBuiltin)
	require.True(t, ok)
	require.Equal(t, "provider", action.Name)
}

func TestPaletteBackgroundDiscoveryPreservesTypedCommand(t *testing.T) {
	c := commandContractDialog(t)
	for _, r := range "privacy-settings" {
		c.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	c.HandleMsg(dockerMCPAvailabilityCheckedMsg{available: false})
	for _, r := range " off" {
		c.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	action, ok := c.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter}).(ActionSlashInput)
	require.True(t, ok)
	require.Equal(t, "/privacy-settings off", action.Input)
}
