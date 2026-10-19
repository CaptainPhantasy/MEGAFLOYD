package dialog

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/legacy-ai/floyd/internal/ui/common"
)

// InfoID is the identifier for the read-only information panel.
const InfoID = "info"

// Info is a read-only panel used for /usage, /status, /tasks, /btw and
// other commands whose result is a block of text. It scrolls, so long
// outputs (such as a side-answer) stay readable.
type Info struct {
	com   *common.Common
	help  help.Model
	title string
	body  string

	width  int
	height int
	offset int

	keyMap struct {
		Up,
		Down,
		PageUp,
		PageDown,
		Home,
		End,
		Close key.Binding
	}
}

var _ Dialog = (*Info)(nil)

// NewInfo creates a read-only information panel.
func NewInfo(com *common.Common, title, body string) *Info {
	m := &Info{com: com, title: title, body: body}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	m.help = h

	m.keyMap.Up = key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑", "scroll up"),
	)
	m.keyMap.Down = key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓", "scroll down"),
	)
	m.keyMap.PageUp = key.NewBinding(
		key.WithKeys("pgup", "b"),
		key.WithHelp("pgup", "page up"),
	)
	m.keyMap.PageDown = key.NewBinding(
		key.WithKeys("pgdown", "space", "f"),
		key.WithHelp("pgdn", "page down"),
	)
	m.keyMap.Home = key.NewBinding(
		key.WithKeys("home", "g"),
	)
	m.keyMap.End = key.NewBinding(
		key.WithKeys("end", "G"),
	)
	m.keyMap.Close = CloseKey

	return m
}

// ID implements Dialog.
func (m *Info) ID() string { return InfoID }

// Title returns the panel title.
func (m *Info) Title() string { return m.title }

// Body returns the panel contents.
func (m *Info) Body() string { return m.body }

// HandleMsg implements Dialog.
func (m *Info) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	switch {
	case key.Matches(keyMsg, m.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, m.keyMap.Up):
		m.scrollBy(-1)
	case key.Matches(keyMsg, m.keyMap.Down):
		m.scrollBy(1)
	case key.Matches(keyMsg, m.keyMap.PageUp):
		m.scrollBy(-m.visibleLines())
	case key.Matches(keyMsg, m.keyMap.PageDown):
		m.scrollBy(m.visibleLines())
	case key.Matches(keyMsg, m.keyMap.Home):
		m.offset = 0
	case key.Matches(keyMsg, m.keyMap.End):
		m.offset = m.maxOffset()
	}
	return nil
}

func (m *Info) scrollBy(delta int) {
	m.offset = max(0, min(m.maxOffset(), m.offset+delta))
}

// Cursor implements Dialog.
func (m *Info) Cursor() *tea.Cursor { return nil }

// Draw implements Dialog.
func (m *Info) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	m.width = width
	m.height = height

	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	bodyStyle := t.Dialog.PrimaryText.Width(max(0, innerWidth))
	wrapped := bodyStyle.Render(m.body)
	m.offset = max(0, min(m.maxOffset(), m.offset))
	lines := strings.Split(wrapped, "\n")
	visible := m.visibleLines()
	end := min(len(lines), m.offset+visible)

	rc := NewRenderContext(t, width)
	rc.Title = m.title
	if len(lines) > visible {
		rc.AddPart(t.Dialog.SecondaryText.Render(
			ansi.Truncate("… scroll for more", innerWidth, "…"),
		))
	}
	rc.AddPart(strings.Join(lines[m.offset:end], "\n"))
	rc.Help = renderDialogHelp(t, &m.help, m, innerWidth)

	view := rc.Render()
	DrawCenterCursor(scr, area, view, nil)
	return nil
}

func (m *Info) visibleLines() int {
	frame := m.com.Styles.Dialog.Title.GetVerticalFrameSize() +
		m.com.Styles.Dialog.HelpView.GetVerticalFrameSize() +
		m.com.Styles.Dialog.View.GetVerticalFrameSize() + 2
	return max(1, m.height-frame)
}

func (m *Info) maxOffset() int {
	innerWidth := max(0, m.width-m.com.Styles.Dialog.View.GetHorizontalFrameSize())
	wrapped := m.com.Styles.Dialog.PrimaryText.Width(innerWidth).Render(m.body)
	lines := strings.Split(wrapped, "\n")
	return max(0, len(lines)-m.visibleLines())
}

// ShortHelp implements help.KeyMap.
func (m *Info) ShortHelp() []key.Binding {
	if m.maxOffset() == 0 {
		return []key.Binding{m.keyMap.Close}
	}
	return []key.Binding{m.keyMap.Up, m.keyMap.Down, m.keyMap.Close}
}

// FullHelp implements help.KeyMap.
func (m *Info) FullHelp() [][]key.Binding {
	return [][]key.Binding{{
		m.keyMap.Up, m.keyMap.Down, m.keyMap.PageUp, m.keyMap.PageDown, m.keyMap.Close,
	}}
}
