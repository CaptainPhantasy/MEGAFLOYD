package model

import (
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

type paletteRuntimeProbe struct {
	command tea.Cmd
	seen    []int
}

func (p *paletteRuntimeProbe) Init() tea.Cmd  { return p.command }
func (p *paletteRuntimeProbe) View() tea.View { return tea.NewView("") }
func (p *paletteRuntimeProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if value, ok := msg.(int); ok {
		p.seen = append(p.seen, value)
		if len(p.seen) == 2 {
			return p, tea.Quit
		}
	}
	return p, nil
}

func TestPaletteFrameworkCommandsExecuteInRealEventLoop(t *testing.T) {
	for _, name := range []string{"sequence", "batch", "quit"} {
		t.Run(name, func(t *testing.T) {
			m, overlay := newBuiltinTestUI(testConfigWithModel(1000))
			palette, err := dialog.NewCommands(m.com, "s1", true, false, false, nil, nil)
			require.NoError(t, err)
			overlay.OpenDialog(palette)
			one := func() tea.Msg { return 1 }
			two := func() tea.Msg { return 2 }
			command := tea.Sequence(one, two)
			if name == "batch" {
				command = tea.Batch(one, two)
			}
			if name == "quit" {
				command = tea.Quit
			}
			probe := &paletteRuntimeProbe{command: m.finishPaletteDispatch(command)}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			_, err = tea.NewProgram(probe, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler()).Run()
			require.NoError(t, err)
			if name == "sequence" {
				require.Equal(t, []int{1, 2}, probe.seen)
			}
			if name == "batch" {
				require.ElementsMatch(t, []int{1, 2}, probe.seen)
			}
		})
	}
}
