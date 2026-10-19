package dialog

import (
	"context"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// RenameSessionID is the identifier for the rename session dialog.
const RenameSessionID = "rename_session"

// RenameSession is a dialog for renaming the current session.
type RenameSession struct {
	com     *common.Common
	help    help.Model
	input   textinput.Model
	session session.Session

	keyMap struct {
		Submit key.Binding
		Close  key.Binding
	}
}

var _ Dialog = (*RenameSession)(nil)

// NewRenameSession creates a new rename session dialog.
func NewRenameSession(com *common.Common, sess session.Session) (*RenameSession, tea.Cmd) {
	m := &RenameSession{
		com:     com,
		session: sess,
	}

	m.input = textinput.New()
	m.input.SetVirtualCursor(false)
	m.input.Placeholder = "Enter session title..."
	m.input.SetValue(sess.Title)
	m.input.SetStyles(com.Styles.TextInput)
	m.input.Focus()
	m.input.SetWidth(40)

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	m.help = help

	m.keyMap.Submit = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "rename"),
	)
	m.keyMap.Close = CloseKey

	return m, nil
}

// ID implements Dialog.
func (m *RenameSession) ID() string {
	return RenameSessionID
}

// HandleMsg implements Dialog.
func (m *RenameSession) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, m.keyMap.Submit):
			return m.confirmRename()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return ActionCmd{cmd}
		}
	}
	return nil
}

func (m *RenameSession) confirmRename() Action {
	newTitle := m.input.Value()
	if newTitle == "" {
		return ActionClose{}
	}

	m.session.Title = newTitle
	return ActionCmd{m.renameSessionCmd()}
}

func (m *RenameSession) renameSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if _, err := m.com.Workspace.SaveSession(context.Background(), m.session); err != nil {
			return util.NewErrorMsg(err)
		}
		return util.NewInfoMsg("Session renamed to: " + m.session.Title)
	}
}

// Cursor returns cursor position.
func (m *RenameSession) Cursor() *tea.Cursor {
	return InputCursor(m.com.Styles, m.input.Cursor())
}

// Draw implements Dialog.
func (m *RenameSession) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize() - 2

	m.input.SetWidth(max(0, innerWidth-t.Dialog.InputPrompt.GetHorizontalFrameSize()-1))
	m.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "Rename Session"
	rc.Help = renderDialogHelp(t, &m.help, m, innerWidth)

	inputView := t.Dialog.InputPrompt.Render(m.input.View())
	rc.AddPart(inputView)

	view := rc.Render()
	DrawCenterCursor(scr, area, view, m.Cursor())
	return m.Cursor()
}

// ShortHelp implements help.KeyMap.
func (m *RenameSession) ShortHelp() []key.Binding {
	return []key.Binding{
		m.keyMap.Submit,
		m.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (m *RenameSession) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}
