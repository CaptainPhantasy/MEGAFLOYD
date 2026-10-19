package dialog

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/ui/styles"
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
