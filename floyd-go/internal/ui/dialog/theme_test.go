package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func TestThemePickerSelectsNextTheme(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	com := &common.Common{Styles: &st}
	d := NewTheme(com, styles.ThemeDark)

	item, ok := d.list.SelectedItem().(*ThemeItem)
	require.True(t, ok)
	require.Equal(t, styles.ThemeDark, item.ID())

	require.Nil(t, d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyDown}))
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	selected, ok := action.(ActionSelectTheme)
	require.True(t, ok)
	require.Equal(t, styles.ThemeLight, selected.Name)
}

func TestThemePickerClosesOnEscape(t *testing.T) {
	t.Parallel()

	st := styles.CharmtonePantera()
	com := &common.Common{Styles: &st}
	d := NewTheme(com, styles.ThemeAuto)

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, ok := action.(ActionClose)
	require.True(t, ok)
}
