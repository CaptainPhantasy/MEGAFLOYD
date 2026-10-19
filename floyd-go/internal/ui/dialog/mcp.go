package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/sahilm/fuzzy"
)

const (
	// MCPServersID is the identifier for the MCP servers dialog.
	MCPServersID       = "mcp_servers"
	mcpDialogMaxWidth  = 80
	mcpDialogMaxHeight = 20
)

// MCPServers represents a dialog for toggling MCP servers.
type MCPServers struct {
	com   *common.Common
	help  help.Model
	list  *list.FilterableList
	input textinput.Model

	keyMap struct {
		Select   key.Binding
		Toggle   key.Binding
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Close    key.Binding
	}
}

// MCPServerItem represents an MCP server list item.
type MCPServerItem struct {
	*list.Versioned
	name     string
	disabled bool
	t        *styles.Styles
	m        fuzzy.Match
	cache    map[int]string
	focused  bool
}

var (
	_ Dialog   = (*MCPServers)(nil)
	_ ListItem = (*MCPServerItem)(nil)
)

// NewMCPServers creates a new MCP servers dialog.
func NewMCPServers(com *common.Common) (*MCPServers, error) {
	m := &MCPServers{com: com}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	m.help = help

	m.list = list.NewFilterableList()
	m.list.Focus()

	m.input = textinput.New()
	m.input.SetVirtualCursor(false)
	m.input.Placeholder = "Type to filter"
	m.input.SetStyles(com.Styles.TextInput)
	m.input.Focus()

	m.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "toggle"),
	)
	m.keyMap.Toggle = key.NewBinding(
		// NOTE: Key.String() reports the spacebar as "space", never " ".
		// Binding only " " makes this dead while help claims space toggles.
		key.WithKeys("space", " "),
		key.WithHelp("space", "toggle"),
	)
	m.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next item"),
	)
	m.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	m.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	m.keyMap.Close = CloseKey

	m.setMCPItems()

	return m, nil
}

// ID implements Dialog.
func (m *MCPServers) ID() string {
	return MCPServersID
}

// HandleMsg implements [Dialog].
func (m *MCPServers) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, m.keyMap.Previous):
			m.list.Focus()
			if m.list.IsSelectedFirst() {
				m.list.SelectLast()
				m.list.ScrollToBottom()
				break
			}
			m.list.SelectPrev()
			m.list.ScrollToSelected()
		case key.Matches(msg, m.keyMap.Next):
			m.list.Focus()
			if m.list.IsSelectedLast() {
				m.list.SelectFirst()
				m.list.ScrollToTop()
				break
			}
			m.list.SelectNext()
			m.list.ScrollToSelected()
		case key.Matches(msg, m.keyMap.Select), key.Matches(msg, m.keyMap.Toggle):
			selectedItem := m.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			mcpItem, ok := selectedItem.(*MCPServerItem)
			if !ok {
				break
			}
			return ActionToggleMCP{Name: mcpItem.name}
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			value := m.input.Value()
			m.list.SetFilter(value)
			m.list.ScrollToTop()
			m.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position relative to the dialog.
func (m *MCPServers) Cursor() *tea.Cursor {
	return InputCursor(m.com.Styles, m.input.Cursor())
}

// Draw implements [Dialog].
func (m *MCPServers) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	width := max(0, min(mcpDialogMaxWidth, area.Dx()))
	height := max(0, min(mcpDialogMaxHeight, area.Dy()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	m.input.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
	m.list.SetSize(innerWidth, height-heightOffset)
	m.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "MCP Servers"
	inputView := t.Dialog.InputPrompt.Render(m.input.View())
	rc.AddPart(inputView)

	visibleCount := len(m.list.FilteredItems())
	if m.list.Height() >= visibleCount {
		m.list.ScrollToTop()
	} else {
		m.list.ScrollToSelected()
	}

	listView := t.Dialog.List.Height(m.list.Height()).Render(m.list.Render())
	rc.AddPart(listView)
	rc.Help = renderDialogHelp(t, &m.help, m, innerWidth)

	view := rc.Render()

	cur := m.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (m *MCPServers) ShortHelp() []key.Binding {
	return []key.Binding{
		m.keyMap.UpDown,
		m.keyMap.Toggle,
		m.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (m *MCPServers) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.keyMap.Toggle, m.keyMap.Next, m.keyMap.Previous, m.keyMap.Close},
	}
}

func (m *MCPServers) setMCPItems() {
	cfg := m.com.Config()
	items := make([]list.FilterableItem, 0, len(cfg.MCP))

	for _, mcp := range cfg.MCP.Sorted() {
		item := &MCPServerItem{
			Versioned: list.NewVersioned(),
			name:      mcp.Name,
			disabled:  mcp.MCP.Disabled,
			t:         m.com.Styles,
		}
		items = append(items, item)
	}

	m.list.SetItems(items...)
	m.list.SetSelected(0)
	m.list.ScrollToTop()
}

// Refresh reloads the MCP items from config.
func (m *MCPServers) Refresh() {
	m.setMCPItems()
}

// Filter returns the filter value for the MCP server item.
// Finished implements list.Item. Library items are render-stable
// outside of explicit SetFocused / SetMatch calls, which invalidate their
// internal render cache.
func (m *MCPServerItem) Finished() bool {
	return true
}

func (m *MCPServerItem) Filter() string {
	return m.name
}

// ID returns the unique identifier for the MCP server.
func (m *MCPServerItem) ID() string {
	return m.name
}

// SetFocused sets the focus state of the MCP server item.
func (m *MCPServerItem) SetFocused(focused bool) {
	if m.focused == focused {
		return
	}
	m.cache = nil
	m.focused = focused
	if m.Versioned != nil {
		m.Bump()
	}
}

// SetMatch sets the fuzzy match for the MCP server item.
func (m *MCPServerItem) SetMatch(match fuzzy.Match) {
	if sameFuzzyMatch(m.m, match) {
		return
	}
	m.cache = nil
	m.m = match
	if m.Versioned != nil {
		m.Bump()
	}
}

// Render returns the string representation of the MCP server item.
func (m *MCPServerItem) Render(width int) string {
	status := "enabled"
	if m.disabled {
		status = "disabled"
	}
	styles := ListItemStyles{
		ItemBlurred:     m.t.Dialog.NormalItem,
		ItemFocused:     m.t.Dialog.SelectedItem,
		InfoTextBlurred: m.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: m.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(styles, m.name, status, m.focused, width, m.cache, &m.m)
}
