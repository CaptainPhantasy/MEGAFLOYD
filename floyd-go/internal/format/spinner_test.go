package format

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/anim"
	"github.com/stretchr/testify/require"
)

func TestSpinnerCancellationKeysCancelWorkAndQuit(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.Key{{Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEscape}} {
		cancelled := false
		m := model{cancel: func() { cancelled = true }, anim: anim.New(anim.Settings{})}
		_, cmd := m.Update(tea.KeyPressMsg(key))
		require.True(t, cancelled)
		require.NotNil(t, cmd)
		require.IsType(t, tea.QuitMsg{}, cmd())
	}
	cancelled := false
	m := model{cancel: func() { cancelled = true }, anim: anim.New(anim.Settings{})}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.False(t, cancelled)
	require.Nil(t, cmd)
	require.NotNil(t, m.Init())
}
