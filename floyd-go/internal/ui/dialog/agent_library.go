package dialog

import (
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"

	"github.com/legacy-ai/floyd/internal/agents"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
)

const (
	// AgentLibraryID is the identifier for the agent library dialog.
	AgentLibraryID              = "agent_library"
	agentLibraryDialogMaxWidth  = 80
	agentLibraryDialogMaxHeight = 24
)

// AgentCategory represents a category tab in the agent library.
type AgentCategory struct {
	Name  string
	Key   string // "0", "1", "2", etc.
	Count int
}

// AgentLibrary represents a dialog for selecting agents from the library.
type AgentLibrary struct {
	com    *common.Common
	help   help.Model
	list   *list.FilterableList
	input  textinput.Model
	agents []agents.AgentDefinition

	// Category system
	categories       []AgentCategory
	selectedCategory int // 0 = All
	categoryCounts   map[string]int

	filterTimer *time.Timer

	keyMap struct {
		Select       key.Binding
		Next         key.Binding
		Previous     key.Binding
		UpDown       key.Binding
		Close        key.Binding
		CategoryNext key.Binding
		CategoryPrev key.Binding
		Category0    key.Binding
		Category1    key.Binding
		Category2    key.Binding
		Category3    key.Binding
		Category4    key.Binding
		Category5    key.Binding
		Category6    key.Binding
		Category7    key.Binding
		Category8    key.Binding
		Category9    key.Binding
		FocusFilter  key.Binding
		Expand       key.Binding
	}
}

// AgentLibraryItem represents an agent list item.
type AgentLibraryItem struct {
	*list.Versioned
	agent    agents.AgentDefinition
	t        *styles.Styles
	m        fuzzy.Match
	cache    map[int]string
	focused  bool
	expanded bool
}

var (
	_ Dialog   = (*AgentLibrary)(nil)
	_ ListItem = (*AgentLibraryItem)(nil)
)

// NewAgentLibrary creates a new agent library dialog.
func NewAgentLibrary(com *common.Common, agentsDirs []string) (*AgentLibrary, error) {
	a := &AgentLibrary{com: com}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	a.help = help

	a.list = list.NewFilterableList()
	a.list.Focus()

	a.input = textinput.New()
	a.input.SetVirtualCursor(false)
	a.input.Placeholder = "Type to filter agents..."
	a.input.SetStyles(com.Styles.TextInput)
	a.input.Focus()

	// Key bindings
	a.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "select"),
	)
	a.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next"),
	)
	a.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "prev"),
	)
	a.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	a.keyMap.Close = CloseKey
	a.keyMap.CategoryNext = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next category"),
	)
	a.keyMap.CategoryPrev = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("⇧+tab", "prev category"),
	)
	a.keyMap.Category0 = key.NewBinding(key.WithKeys("0"), key.WithHelp("0", "all"))
	a.keyMap.Category1 = key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "architecture"))
	a.keyMap.Category2 = key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "infrastructure"))
	a.keyMap.Category3 = key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "orchestration"))
	a.keyMap.Category4 = key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "coding"))
	a.keyMap.Category5 = key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "security"))
	a.keyMap.Category6 = key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "quality"))
	a.keyMap.Category7 = key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "testing"))
	a.keyMap.Category8 = key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "monitoring"))
	a.keyMap.Category9 = key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "dx"))
	a.keyMap.FocusFilter = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	)
	a.keyMap.Expand = key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "expand"),
	)

	a.loadAgents(agentsDirs)

	return a, nil
}

// loadAgents loads agents from the specified directories.
func (a *AgentLibrary) loadAgents(dirs []string) {
	a.agents = []agents.AgentDefinition{}
	a.categoryCounts = make(map[string]int)

	// Track seen agents to avoid duplicates (preferring project-local over global)
	seen := make(map[string]bool)

	for _, dir := range dirs {
		loaded, err := agents.LoadAgents(dir)
		if err == nil {
			for _, agent := range loaded {
				if seen[agent.Name] {
					slog.Debug("Skipping duplicate agent", "name", agent.Name, "path", agent.FilePath)
					continue
				}
				a.agents = append(a.agents, agent)
				seen[agent.Name] = true
				// Count by category
				if agent.Category != "" {
					a.categoryCounts[agent.Category]++
				}
			}
		}
	}

	// Build category tabs
	a.buildCategories()
	a.updateListItems()
}

// buildCategories creates the category tab list.
func (a *AgentLibrary) buildCategories() {
	// Define category order (matching key bindings 1-9)
	categoryOrder := []string{
		"architecture",
		"infrastructure",
		"orchestration",
		"coding",
		"security",
		"quality",
		"testing",
		"monitoring",
		"dx",
	}

	// Always start with "All"
	a.categories = []AgentCategory{
		{Name: "All", Key: "0", Count: len(a.agents)},
	}

	// Add categories that have agents
	for i, cat := range categoryOrder {
		if count := a.categoryCounts[cat]; count > 0 {
			a.categories = append(a.categories, AgentCategory{
				Name:  cat,
				Key:   string(rune('1' + i)),
				Count: count,
			})
		}
	}
}

// updateListItems updates the list items based on selected category.
func (a *AgentLibrary) updateListItems() {
	var filtered []agents.AgentDefinition

	if a.selectedCategory == 0 {
		// Show all agents
		filtered = a.agents
	} else {
		// Filter by category
		cat := a.categories[a.selectedCategory].Name
		for _, agent := range a.agents {
			if agent.Category == cat {
				filtered = append(filtered, agent)
			}
		}
	}

	items := make([]list.FilterableItem, 0, len(filtered))
	for _, agent := range filtered {
		items = append(items, &AgentLibraryItem{
			Versioned: list.NewVersioned(),
			agent:     agent,
			t:         a.com.Styles,
		})
	}
	a.list.SetItems(items...)
	a.list.SetFilter(a.input.Value())
	a.list.ScrollToTop()
	a.list.SetSelected(0)
}

// selectCategory selects a category by index.
func (a *AgentLibrary) selectCategory(idx int) {
	if idx >= 0 && idx < len(a.categories) {
		a.selectedCategory = idx
		a.updateListItems()
	}
}

// categoryTabBar renders the category tab bar.
func (a *AgentLibrary) categoryTabBar(width int) string {
	t := a.com.Styles
	var tabs []string

	for i, cat := range a.categories {
		var tab string
		label := cat.Name
		if cat.Count > 0 {
			label = cat.Name
		}

		if i == a.selectedCategory {
			// Selected tab
			tab = t.Dialog.PrimaryText.Render("● ") + t.Dialog.PrimaryText.Bold(true).Render(label)
		} else {
			// Unselected tab
			tab = t.Dialog.SecondaryText.Render(label)
		}
		tabs = append(tabs, tab)
	}

	return strings.Join(tabs, t.Dialog.SecondaryText.Render(" │ "))
}

// ID implements Dialog.
func (a *AgentLibrary) ID() string {
	return AgentLibraryID
}

// HandleMsg implements Dialog.
func (a *AgentLibrary) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, a.keyMap.Close):
			return ActionClose{}

		case key.Matches(msg, a.keyMap.CategoryNext):
			a.selectCategory((a.selectedCategory + 1) % len(a.categories))

		case key.Matches(msg, a.keyMap.CategoryPrev):
			newIdx := a.selectedCategory - 1
			if newIdx < 0 {
				newIdx = len(a.categories) - 1
			}
			a.selectCategory(newIdx)

		case key.Matches(msg, a.keyMap.Category0):
			a.selectCategory(0)
		case key.Matches(msg, a.keyMap.Category1) && len(a.categories) > 1:
			a.selectCategory(1)
		case key.Matches(msg, a.keyMap.Category2) && len(a.categories) > 2:
			a.selectCategory(2)
		case key.Matches(msg, a.keyMap.Category3) && len(a.categories) > 3:
			a.selectCategory(3)
		case key.Matches(msg, a.keyMap.Category4) && len(a.categories) > 4:
			a.selectCategory(4)
		case key.Matches(msg, a.keyMap.Category5) && len(a.categories) > 5:
			a.selectCategory(5)
		case key.Matches(msg, a.keyMap.Category6) && len(a.categories) > 6:
			a.selectCategory(6)
		case key.Matches(msg, a.keyMap.Category7) && len(a.categories) > 7:
			a.selectCategory(7)
		case key.Matches(msg, a.keyMap.Category8) && len(a.categories) > 8:
			a.selectCategory(8)
		case key.Matches(msg, a.keyMap.Category9) && len(a.categories) > 9:
			a.selectCategory(9)

		case key.Matches(msg, a.keyMap.Previous):
			a.list.Focus()
			if a.list.IsSelectedFirst() {
				a.list.SelectLast()
				a.list.ScrollToBottom()
				break
			}
			a.list.SelectPrev()
			a.list.ScrollToSelected()

		case key.Matches(msg, a.keyMap.Next):
			a.list.Focus()
			if a.list.IsSelectedLast() {
				a.list.SelectFirst()
				a.list.ScrollToTop()
				break
			}
			a.list.SelectNext()
			a.list.ScrollToSelected()

		case key.Matches(msg, a.keyMap.Select):
			selectedItem := a.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			agentItem, ok := selectedItem.(*AgentLibraryItem)
			if !ok {
				break
			}
			return ActionSelectAgent{
				AgentName:        agentItem.agent.Name,
				AgentDescription: agentItem.agent.Description,
				SystemPrompt:     agentItem.agent.SystemPrompt,
			}

		case key.Matches(msg, a.keyMap.FocusFilter):
			a.input.Focus()

		case key.Matches(msg, a.keyMap.Expand):
			selectedItem := a.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			if agentItem, ok := selectedItem.(*AgentLibraryItem); ok {
				agentItem.expanded = !agentItem.expanded
				agentItem.cache = nil // Clear cache to re-render
				agentItem.Bump()
			}

		default:
			var cmd tea.Cmd
			a.input, cmd = a.input.Update(msg)
			value := a.input.Value()
			a.list.SetFilter(value)
			a.list.ScrollToTop()
			a.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns the cursor position.
func (a *AgentLibrary) Cursor() *tea.Cursor {
	return InputCursor(a.com.Styles, a.input.Cursor())
}

// Draw implements Dialog.
func (a *AgentLibrary) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := a.com.Styles
	width := max(0, min(agentLibraryDialogMaxWidth, area.Dx()))
	height := max(0, min(agentLibraryDialogMaxHeight, area.Dy()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	// Height calculation includes category tab bar
	categoryBarHeight := 1
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		categoryBarHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	a.input.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
	a.list.SetSize(innerWidth, height-heightOffset)
	a.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)

	// Title with count
	totalCount := len(a.agents)
	catName := a.categories[a.selectedCategory].Name
	if catName == "All" {
		rc.Title = "Agent Library"
		rc.TitleInfo = t.Dialog.SecondaryText.Render(strings.Join([]string{"", string(rune(countDigits(totalCount))), " agents"}, ""))
	} else {
		rc.Title = "Agent Library"
		rc.TitleInfo = t.Dialog.SecondaryText.Render(" " + catName)
	}

	// Category tab bar
	categoryBar := a.categoryTabBar(innerWidth)
	rc.AddPart(categoryBar)

	// Filter input
	inputView := t.Dialog.InputPrompt.Render(a.input.View())
	rc.AddPart(inputView)

	// List
	visibleCount := len(a.list.FilteredItems())
	if visibleCount == 0 {
		emptyView := "\n\n" + t.Dialog.SecondaryText.Render("  No agents found matching your filter.") + "\n"
		emptyView += t.Dialog.SecondaryText.Render("  Try a broader term or check other categories.")
		rc.AddPart(emptyView)
	} else {
		if a.list.Height() >= visibleCount {
			a.list.ScrollToTop()
		} else {
			a.list.ScrollToSelected()
		}
		listView := t.Dialog.List.Height(a.list.Height()).Render(a.list.Render())
		rc.AddPart(listView)
	}

	rc.Help = renderDialogHelp(t, &a.help, a, innerWidth)

	view := rc.Render()

	cur := a.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

func countDigits(n int) int {
	if n == 0 {
		return 1
	}
	count := 0
	for n > 0 {
		n /= 10
		count++
	}
	return count
}

// ShortHelp implements help.KeyMap.
func (a *AgentLibrary) ShortHelp() []key.Binding {
	return []key.Binding{
		a.keyMap.UpDown,
		a.keyMap.CategoryNext,
		a.keyMap.Select,
		a.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (a *AgentLibrary) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{a.keyMap.Select, a.keyMap.Next, a.keyMap.Previous, a.keyMap.FocusFilter},
		{a.keyMap.CategoryNext, a.keyMap.CategoryPrev, a.keyMap.Close},
	}
}

// Filter implements ListItem.
// Finished implements list.Item. Library items are render-stable
// outside of explicit SetFocused / SetMatch calls, which invalidate their
// internal render cache.
func (a *AgentLibraryItem) Finished() bool {
	return true
}

func (a *AgentLibraryItem) Filter() string {
	// Structural indexing locked to visible string rendering to prevent fuzzy match panic
	return a.agent.Name + " • " + a.agent.Description
}

// ID implements ListItem.
func (a *AgentLibraryItem) ID() string {
	return a.agent.Name
}

// SetFocused implements ListItem.
func (a *AgentLibraryItem) SetFocused(focused bool) {
	if a.focused == focused {
		return
	}
	a.cache = nil
	a.focused = focused
	if a.Versioned != nil {
		a.Bump()
	}
}

// SetMatch implements ListItem.
func (a *AgentLibraryItem) SetMatch(match fuzzy.Match) {
	if sameFuzzyMatch(a.m, match) {
		return
	}
	a.cache = nil
	a.m = match
	if a.Versioned != nil {
		a.Bump()
	}
}

// Render implements ListItem.
func (a *AgentLibraryItem) Render(width int) string {
	styles := ListItemStyles{
		ItemBlurred:     a.t.Dialog.NormalItem,
		ItemFocused:     a.t.Dialog.SelectedItem.Bold(true),
		InfoTextBlurred: a.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: a.t.Dialog.ListItem.InfoFocused,
	}

	displayInfo := ""
	if a.agent.Category != "" {
		displayInfo = " [" + a.agent.Category + "]"
	}

	var metaStyle string
	if a.focused {
		metaStyle = a.t.Dialog.SecondaryText.Render(" • " + a.agent.Description)
	} else {
		metaStyle = a.t.Dialog.SecondaryText.Render(" • " + a.agent.Description)
	}
	displayTitle := a.agent.Name + metaStyle

	if a.expanded {
		// Show preview of system prompt when expanded
		preview := a.agent.SystemPrompt
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		// NOTE: Append preview safely. renderItem applies to first line alignment.
		displayTitle = displayTitle + "\n\n" + a.t.Dialog.SecondaryText.Render(preview)
	}

	return renderItem(styles, displayTitle, displayInfo, a.focused, width, a.cache, &a.m)
}
