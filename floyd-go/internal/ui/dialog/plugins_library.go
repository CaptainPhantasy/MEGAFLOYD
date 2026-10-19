package dialog

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"

	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
)

const (
	// PluginsLibraryID is the identifier for the plugins library dialog.
	PluginsLibraryID              = "plugins_library"
	pluginsLibraryDialogMaxWidth  = 80
	pluginsLibraryDialogMaxHeight = 24
)

// PluginStatus represents the status of a plugin.
type PluginStatus int

const (
	PluginStatusConnected PluginStatus = iota
	PluginStatusConfigured
	PluginStatusAvailable
	PluginStatusError
)

// PluginInfo represents information about a plugin/MCP server.
type PluginInfo struct {
	Name        string
	Description string
	Status      PluginStatus
	ToolsCount  int
	Error       string
	Type        string // stdio, http, etc.
}

// PluginsLibrary represents a dialog for managing plugins (MCP servers).
type PluginsLibrary struct {
	com     *common.Common
	help    help.Model
	list    *list.FilterableList
	input   textinput.Model
	plugins []PluginInfo

	// Category system
	categories       []string
	selectedCategory int

	keyMap struct {
		Select       key.Binding
		Next         key.Binding
		Previous     key.Binding
		UpDown       key.Binding
		Close        key.Binding
		CategoryNext key.Binding
		CategoryPrev key.Binding
		FocusFilter  key.Binding
		Connect      key.Binding
	}
}

// PluginsLibraryItem represents a plugin list item.
type PluginsLibraryItem struct {
	*list.Versioned
	plugin  PluginInfo
	t       *styles.Styles
	m       fuzzy.Match
	cache   map[int]string
	focused bool
}

var (
	_ Dialog   = (*PluginsLibrary)(nil)
	_ ListItem = (*PluginsLibraryItem)(nil)
)

// NewPluginsLibrary creates a new plugins library dialog.
func NewPluginsLibrary(com *common.Common) (*PluginsLibrary, error) {
	p := &PluginsLibrary{com: com}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	p.help = help

	p.list = list.NewFilterableList()
	p.list.Focus()

	p.input = textinput.New()
	p.input.SetVirtualCursor(false)
	p.input.Placeholder = "Type to filter plugins..."
	p.input.SetStyles(com.Styles.TextInput)
	p.input.Focus()

	// Categories for plugins
	p.categories = []string{"All", "Connected", "Configured", "Available"}
	p.selectedCategory = 0

	// Key bindings
	p.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "select"),
	)
	p.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next"),
	)
	p.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "prev"),
	)
	p.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	p.keyMap.Close = CloseKey
	p.keyMap.CategoryNext = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next cat"),
	)
	p.keyMap.CategoryPrev = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("⇧+tab", "prev cat"),
	)
	p.keyMap.FocusFilter = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	)
	p.keyMap.Connect = key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "configure"),
	)

	p.loadPlugins()

	return p, nil
}

// loadPlugins loads plugin information from the actual configuration.
func (p *PluginsLibrary) loadPlugins() {
	p.plugins = []PluginInfo{}
	cfg := p.com.Config()

	// Get live tool counts from MCP registry
	toolCounts := make(map[string]int)
	for name, tools := range mcp.Tools() {
		toolCounts[name] = len(tools)
	}

	// 1. Load from configured MCP servers
	for name, mcpCfg := range cfg.MCP {
		count := toolCounts[name]
		status := PluginStatusConfigured
		if count > 0 {
			status = PluginStatusConnected
		}

		p.plugins = append(p.plugins, PluginInfo{
			Name:        name,
			Description: fmt.Sprintf("MCP Server (%s)", mcpCfg.Type),
			Status:      status,
			Type:        string(mcpCfg.Type),
			ToolsCount:  count,
		})
	}

	// 2. Add "Available" suggestions (standard MCP servers the user might want)
	available := []PluginInfo{
		{Name: "google-maps", Description: "Search and navigate geographical data", Status: PluginStatusAvailable},
		{Name: "slack", Description: "Interact with Slack channels and messages", Status: PluginStatusAvailable},
		{Name: "linear", Description: "Manage Linear issues and projects", Status: PluginStatusAvailable},
	}

	for _, avail := range available {
		// Only add if not already configured
		found := false
		for _, conf := range p.plugins {
			if conf.Name == avail.Name {
				found = true
				break
			}
		}
		if !found {
			p.plugins = append(p.plugins, avail)
		}
	}

	// Sort by name
	sort.Slice(p.plugins, func(i, j int) bool {
		return p.plugins[i].Name < p.plugins[j].Name
	})

	p.updateListItems()
}

// updateListItems updates the list items based on selected category.
func (p *PluginsLibrary) updateListItems() {
	var filtered []PluginInfo

	for _, plugin := range p.plugins {
		match := false
		switch p.selectedCategory {
		case 0: // All
			match = true
		case 1: // Connected
			match = plugin.Status == PluginStatusConnected
		case 2: // Configured
			match = plugin.Status == PluginStatusConfigured || plugin.Status == PluginStatusConnected
		case 3: // Available
			match = plugin.Status == PluginStatusAvailable
		}

		if match {
			filtered = append(filtered, plugin)
		}
	}

	items := make([]list.FilterableItem, 0, len(filtered))
	for _, plugin := range filtered {
		items = append(items, &PluginsLibraryItem{
			Versioned: list.NewVersioned(),
			plugin:    plugin,
			t:         p.com.Styles,
		})
	}
	p.list.SetItems(items...)
	p.list.SetFilter(p.input.Value())
	p.list.ScrollToTop()
	p.list.SetSelected(0)
}

// categoryTabBar renders the category tab bar.
func (p *PluginsLibrary) categoryTabBar(width int) string {
	t := p.com.Styles
	var tabs []string

	for i, cat := range p.categories {
		var tab string
		if i == p.selectedCategory {
			tab = t.Dialog.PrimaryText.Render("● ") + t.Dialog.PrimaryText.Bold(true).Render(cat)
		} else {
			tab = t.Dialog.SecondaryText.Render(cat)
		}
		tabs = append(tabs, tab)
	}

	return strings.Join(tabs, t.Dialog.SecondaryText.Render(" │ "))
}

// ID implements Dialog.
func (p *PluginsLibrary) ID() string {
	return PluginsLibraryID
}

// HandleMsg implements Dialog.
func (p *PluginsLibrary) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, p.keyMap.Close):
			return ActionClose{}

		case key.Matches(msg, p.keyMap.CategoryNext):
			p.selectedCategory = (p.selectedCategory + 1) % len(p.categories)
			p.updateListItems()

		case key.Matches(msg, p.keyMap.CategoryPrev):
			newIdx := p.selectedCategory - 1
			if newIdx < 0 {
				newIdx = len(p.categories) - 1
			}
			p.selectedCategory = newIdx
			p.updateListItems()

		case key.Matches(msg, p.keyMap.Previous):
			p.list.Focus()
			if p.list.IsSelectedFirst() {
				p.list.SelectLast()
				p.list.ScrollToBottom()
				break
			}
			p.list.SelectPrev()
			p.list.ScrollToSelected()

		case key.Matches(msg, p.keyMap.Next):
			p.list.Focus()
			if p.list.IsSelectedLast() {
				p.list.SelectFirst()
				p.list.ScrollToTop()
				break
			}
			p.list.SelectNext()
			p.list.ScrollToSelected()

		case key.Matches(msg, p.keyMap.Select):
			selectedItem := p.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			pluginItem, ok := selectedItem.(*PluginsLibraryItem)
			if !ok {
				break
			}
			return ActionSelectPlugin{
				PluginName:        pluginItem.plugin.Name,
				PluginDescription: pluginItem.plugin.Description,
				PluginStatus:      pluginItem.plugin.Status,
			}

		case key.Matches(msg, p.keyMap.FocusFilter):
			p.input.Focus()

		default:
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			value := p.input.Value()
			p.list.SetFilter(value)
			p.list.ScrollToTop()
			p.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns cursor position.
func (p *PluginsLibrary) Cursor() *tea.Cursor {
	return InputCursor(p.com.Styles, p.input.Cursor())
}

// Draw implements Dialog.
func (p *PluginsLibrary) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := p.com.Styles
	width := max(0, min(pluginsLibraryDialogMaxWidth, area.Dx()))
	height := max(0, min(pluginsLibraryDialogMaxHeight, area.Dy()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	categoryBarHeight := 1
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		categoryBarHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	p.input.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
	p.list.SetSize(innerWidth, height-heightOffset)
	p.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "Plugins (MCP Servers)"
	rc.TitleInfo = t.Dialog.SecondaryText.Render(fmt.Sprintf(" %d active", len(p.plugins)))

	categoryBar := p.categoryTabBar(innerWidth)
	rc.AddPart(categoryBar)

	inputView := t.Dialog.InputPrompt.Render(p.input.View())
	rc.AddPart(inputView)

	visibleCount := len(p.list.FilteredItems())
	if p.list.Height() >= visibleCount {
		p.list.ScrollToTop()
	} else {
		p.list.ScrollToSelected()
	}
	listView := t.Dialog.List.Height(p.list.Height()).Render(p.list.Render())
	rc.AddPart(listView)

	rc.Help = renderDialogHelp(t, &p.help, p, innerWidth)

	view := rc.Render()

	cur := p.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements help.KeyMap.
func (p *PluginsLibrary) ShortHelp() []key.Binding {
	return []key.Binding{
		p.keyMap.UpDown,
		p.keyMap.CategoryNext,
		p.keyMap.Select,
		p.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (p *PluginsLibrary) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{p.keyMap.Select, p.keyMap.Next, p.keyMap.Previous, p.keyMap.FocusFilter},
		{p.keyMap.CategoryNext, p.keyMap.CategoryPrev, p.keyMap.Close},
	}
}

// Filter implements ListItem.
// Finished implements list.Item. Library items are render-stable
// outside of explicit SetFocused / SetMatch calls, which invalidate their
// internal render cache.
func (p *PluginsLibraryItem) Finished() bool {
	return true
}

func (p *PluginsLibraryItem) Filter() string {
	return p.plugin.Name + " " + p.plugin.Description
}

// ID implements ListItem.
func (p *PluginsLibraryItem) ID() string {
	return p.plugin.Name
}

// SetFocused implements ListItem.
func (p *PluginsLibraryItem) SetFocused(focused bool) {
	if p.focused == focused {
		return
	}
	p.cache = nil
	p.focused = focused
	if p.Versioned != nil {
		p.Bump()
	}
}

// SetMatch implements ListItem.
func (p *PluginsLibraryItem) SetMatch(match fuzzy.Match) {
	if sameFuzzyMatch(p.m, match) {
		return
	}
	p.cache = nil
	p.m = match
	if p.Versioned != nil {
		p.Bump()
	}
}

// Render implements ListItem.
func (p *PluginsLibraryItem) Render(width int) string {
	styles := ListItemStyles{
		ItemBlurred:     p.t.Dialog.NormalItem,
		ItemFocused:     p.t.Dialog.SelectedItem,
		InfoTextBlurred: p.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: p.t.Dialog.ListItem.InfoFocused,
	}

	var statusIcon string
	switch p.plugin.Status {
	case PluginStatusConnected:
		statusIcon = p.t.Resource.OnlineIcon.String()
	case PluginStatusAvailable:
		statusIcon = p.t.Resource.DisabledIcon.String()
	case PluginStatusConfigured:
		statusIcon = p.t.Resource.BusyIcon.String()
	case PluginStatusError:
		statusIcon = p.t.Resource.ErrorIcon.String()
	}

	name := statusIcon + p.plugin.Name
	desc := p.plugin.Description
	if p.plugin.Type != "" {
		desc = fmt.Sprintf("[%s] %s", p.plugin.Type, desc)
	}

	if p.plugin.ToolsCount > 0 {
		desc = fmt.Sprintf("%s (%d tools)", desc, p.plugin.ToolsCount)
	}

	return renderItem(styles, name, desc, p.focused, width, p.cache, &p.m)
}

// ActionSelectPlugin is a message indicating a plugin has been selected.
type ActionSelectPlugin struct {
	PluginName        string
	PluginDescription string
	PluginStatus      PluginStatus
}
