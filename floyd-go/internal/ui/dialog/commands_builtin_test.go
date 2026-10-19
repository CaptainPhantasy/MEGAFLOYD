package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPaletteItemsCoverRegistry(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	items := builtinPaletteItems(&st)
	require.Len(t, items, len(commands.Builtins()))

	byID := make(map[string]*CommandItem, len(items))
	for _, item := range items {
		byID[item.ID()] = item
	}

	for _, spec := range commands.Builtins() {
		item, ok := byID["cmd_"+spec.Name]
		require.True(t, ok, "missing palette entry for /%s", spec.Name)
		require.Equal(t, "/"+spec.Name, item.title)
		require.Equal(t, "cmd_"+spec.Name, item.ID())

		action, ok := item.Action().(ActionRunBuiltin)
		require.True(t, ok, "%s must dispatch through ActionRunBuiltin", spec.Name)
		require.Equal(t, spec.Name, action.Name)
		require.True(t, action.FromPalette)

		for _, alias := range spec.Aliases {
			require.Contains(t, item.Filter(), alias, "alias %q must be filterable", alias)
		}
	}
}

func TestBlockedAndExcludedCommandsHaveNoPaletteEntry(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	items := builtinPaletteItems(&st)
	ids := make(map[string]bool, len(items))
	for _, item := range items {
		ids[item.ID()] = true
	}

	for name := range commands.BlockedBuiltin {
		require.False(t, ids["cmd_"+name], "blocked /%s must not appear in the palette", name)
	}
	for name := range commands.ExcludedBuiltin {
		require.False(t, ids["cmd_"+name], "excluded /%s must not appear in the palette", name)
	}
}

type commandsTestWorkspace struct {
	workspace.Workspace
	cfg *config.Config
}

func (w *commandsTestWorkspace) Config() *config.Config { return w.cfg }

func TestCommandsDialogDispatchesSlashName(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	com := &common.Common{
		Workspace: &commandsTestWorkspace{cfg: &config.Config{}},
		Styles:    &st,
	}
	d, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)

	for _, r := range "new" {
		d.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	run, ok := action.(ActionRunBuiltin)
	require.True(t, ok, "typing new and pressing enter must dispatch /new")
	require.Equal(t, "new", run.Name)
	require.True(t, run.FromPalette)

	// The same path accepts a typed leading slash in the filter.
	d2, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)
	for _, r := range "/usage" {
		d2.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	action = d2.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	run, ok = action.(ActionRunBuiltin)
	require.True(t, ok, "typing /usage and pressing enter must dispatch /usage")
	require.Equal(t, "usage", run.Name)
}

func TestCommandsDialogDispatchesTypedArguments(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	com := &common.Common{
		Workspace: &commandsTestWorkspace{cfg: &config.Config{}},
		Styles:    &st,
	}

	for _, tc := range []struct {
		typed string
		want  string
	}{
		{"/theme light", "/theme light"},
		{"/plan status", "/plan status"},
		{"/add-dir /tmp", "/add-dir /tmp"},
		{"/experiments ghost_text off", "/experiments ghost_text off"},
		// Opening the palette consumes the leading slash.
		{"theme light", "/theme light"},
		{"plan status", "/plan status"},
	} {
		d, err := NewCommands(com, "", false, false, false, nil, nil)
		require.NoError(t, err)
		for _, r := range tc.typed {
			d.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
		typed, ok := action.(ActionSlashInput)
		require.True(t, ok, "typing %q and pressing enter must dispatch raw slash input", tc.typed)
		require.Equal(t, tc.want, typed.Input)
	}

	// Unknown slash names must reach the model so it can warn instead of
	// silently filtering to nothing.
	d, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)
	for _, r := range "/nope" {
		d.HandleMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	typed, ok := action.(ActionSlashInput)
	require.True(t, ok, "unknown slash commands must dispatch for a warning")
	require.Equal(t, "/nope", typed.Input)
}

func TestPaletteOffersToggleTerminal(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	com := &common.Common{
		Workspace: &commandsTestWorkspace{cfg: &config.Config{}},
		Styles:    &st,
	}
	d, err := NewCommands(com, "", false, false, false, nil, nil)
	require.NoError(t, err)

	for _, item := range d.list.FilteredItems() {
		if ci, ok := item.(*CommandItem); ok && ci.ID() == "toggle_terminal" {
			_, ok := ci.Action().(ActionToggleTerminal)
			require.True(t, ok, "toggle_terminal must use ActionToggleTerminal")
			require.Equal(t, "alt+t", ci.Shortcut())
			return
		}
	}
	t.Fatal("palette is missing the toggle_terminal entry")
}
