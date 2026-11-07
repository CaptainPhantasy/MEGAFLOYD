package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// OpenSessionsMsg opens the session picker after the terminal UI starts.
type OpenSessionsMsg struct{}

type paletteCommandResultMsg struct {
	Parent dialog.Dialog
	Result tea.Msg
}

// finishPaletteDispatch keeps synchronous children on their parent stack. An
// async result restores the same parent only when it actually opens a panel or
// reports an error; prefilled commands never leave an unfinishable loading view.
func (m *UI) finishPaletteDispatch(cmd tea.Cmd) tea.Cmd {
	if m.dialog.FrontID() != dialog.CommandsID {
		return cmd
	}
	parent := m.dialog.DialogLast()
	m.dialog.CloseFrontDialog()
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		result := cmd()
		switch result.(type) {
		case builtinResultMsg, preferenceResultMsg, util.InfoMsg:
			return paletteCommandResultMsg{Parent: parent, Result: result}
		default:
			// Batch, Sequence, Quit and other framework messages must reach
			// Bubble Tea itself; passing them directly to UI.Update drops
			// their commands without executing them.
			return result
		}
	}
}

func (m *UI) restorePaletteParent(msg paletteCommandResultMsg) {
	restore := false
	switch result := msg.Result.(type) {
	case builtinResultMsg:
		restore = result.Title != "" || result.Err != nil
	case preferenceResultMsg:
		restore = result.title != "" || result.err != nil
	case util.InfoMsg:
		restore = result.Type == util.InfoTypeError || result.Type == util.InfoTypeWarn
	}
	if restore && msg.Parent != nil && !m.dialog.ContainsDialog(dialog.CommandsID) {
		m.dialog.OpenDialog(msg.Parent)
	}
}
