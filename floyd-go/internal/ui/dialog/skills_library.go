package dialog

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"

	"github.com/legacy-ai/floyd/internal/skills"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
)

const (
	// SkillsLibraryID is the identifier for the skills library dialog.
	SkillsLibraryID              = "skills_library"
	skillsLibraryDialogMaxWidth  = 80
	skillsLibraryDialogMaxHeight = 24
)

// SkillCategory represents a category tab in the skills library.
type SkillCategory struct {
	Name  string
	Key   string
	Count int
}

// SkillsLibrary represents a dialog for selecting skills from the library.
type SkillsLibrary struct {
	com    *common.Common
	help   help.Model
	list   *list.FilterableList
	input  textinput.Model
	skills []*skills.Skill

	// Category system
	categories       []SkillCategory
	selectedCategory int
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

// SkillsLibraryItem represents a skill list item.
type SkillsLibraryItem struct {
	*list.Versioned
	skill    *skills.Skill
	t        *styles.Styles
	m        fuzzy.Match
	cache    map[int]string
	focused  bool
	expanded bool
}

var (
	_ Dialog   = (*SkillsLibrary)(nil)
	_ ListItem = (*SkillsLibraryItem)(nil)
)

// NewSkillsLibrary creates a new skills library dialog.
func NewSkillsLibrary(com *common.Common, skillsDirs []string) (*SkillsLibrary, error) {
	s := &SkillsLibrary{com: com}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	s.help = help

	s.list = list.NewFilterableList()
	s.list.Focus()

	s.input = textinput.New()
	s.input.SetVirtualCursor(false)
	s.input.Placeholder = "Type to filter skills..."
	s.input.SetStyles(com.Styles.TextInput)
	s.input.Focus()

	// Key bindings
	s.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "select"),
	)
	s.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next"),
	)
	s.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "prev"),
	)
	s.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	s.keyMap.Close = CloseKey
	s.keyMap.CategoryNext = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next category"),
	)
	s.keyMap.CategoryPrev = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("⇧+tab", "prev category"),
	)
	s.keyMap.Category0 = key.NewBinding(key.WithKeys("0"), key.WithHelp("0", "all"))
	s.keyMap.Category1 = key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "git"))
	s.keyMap.Category2 = key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "testing"))
	s.keyMap.Category3 = key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "linting"))
	s.keyMap.Category4 = key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "refactoring"))
	s.keyMap.Category5 = key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "documentation"))
	s.keyMap.Category6 = key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "deployment"))
	s.keyMap.Category7 = key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "security"))
	s.keyMap.Category8 = key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "data"))
	s.keyMap.Category9 = key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "debugging"))
	s.keyMap.FocusFilter = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	)
	s.keyMap.Expand = key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "expand"),
	)

	s.loadSkills(skillsDirs)

	return s, nil
}

// loadSkills loads skills from specified directories.
func (s *SkillsLibrary) loadSkills(dirs []string) {
	s.skills = skills.Discover(dirs)
	s.categoryCounts = make(map[string]int)

	// Count by category
	for _, skill := range s.skills {
		if category := skillCategory(skill); category != "" {
			s.categoryCounts[category]++
		}
	}

	// Build category tabs
	s.buildCategories()
	s.updateListItems()
}

// buildCategories creates the category tab list.
func (s *SkillsLibrary) buildCategories() {
	// Define category order (matching key bindings 1-9)
	categoryOrder := []string{
		"git",
		"testing",
		"linting",
		"refactoring",
		"documentation",
		"deployment",
		"security",
		"data",
		"debugging",
	}

	// Always start with "All"
	s.categories = []SkillCategory{
		{Name: "All", Key: "0", Count: len(s.skills)},
	}

	// Add categories that have skills
	for i, cat := range categoryOrder {
		if count := s.categoryCounts[cat]; count > 0 {
			s.categories = append(s.categories, SkillCategory{
				Name:  cat,
				Key:   string(rune('1' + i)),
				Count: count,
			})
		}
	}
}

// updateListItems updates the list items based on selected category.
func (s *SkillsLibrary) updateListItems() {
	var filtered []*skills.Skill

	if s.selectedCategory == 0 {
		filtered = s.skills
	} else {
		cat := s.categories[s.selectedCategory].Name
		for _, skill := range s.skills {
			if skillCategory(skill) == cat {
				filtered = append(filtered, skill)
			}
		}
	}

	items := make([]list.FilterableItem, 0, len(filtered))
	for _, skill := range filtered {
		items = append(items, &SkillsLibraryItem{
			Versioned: list.NewVersioned(),
			skill:     skill,
			t:         s.com.Styles,
		})
	}
	s.list.SetItems(items...)
	s.list.SetFilter(s.input.Value())
	s.list.ScrollToTop()
	s.list.SetSelected(0)
}

// selectCategory selects a category by index.
func (s *SkillsLibrary) selectCategory(idx int) {
	if idx >= 0 && idx < len(s.categories) {
		s.selectedCategory = idx
		s.updateListItems()
	}
}

// categoryTabBar renders the category tab bar.
func (s *SkillsLibrary) categoryTabBar(width int) string {
	t := s.com.Styles
	var tabs []string

	for i, cat := range s.categories {
		var tab string
		label := cat.Name

		if i == s.selectedCategory {
			tab = t.Dialog.PrimaryText.Render("● ") + t.Dialog.PrimaryText.Bold(true).Render(label)
		} else {
			tab = t.Dialog.SecondaryText.Render(label)
		}
		tabs = append(tabs, tab)
	}

	return strings.Join(tabs, t.Dialog.SecondaryText.Render(" │ "))
}

// ID implements Dialog.
func (s *SkillsLibrary) ID() string {
	return SkillsLibraryID
}

// HandleMsg implements Dialog.
func (s *SkillsLibrary) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, s.keyMap.Close):
			return ActionClose{}

		case key.Matches(msg, s.keyMap.CategoryNext):
			s.selectCategory((s.selectedCategory + 1) % len(s.categories))

		case key.Matches(msg, s.keyMap.CategoryPrev):
			newIdx := s.selectedCategory - 1
			if newIdx < 0 {
				newIdx = len(s.categories) - 1
			}
			s.selectCategory(newIdx)

		case key.Matches(msg, s.keyMap.Category0):
			s.selectCategory(0)
		case key.Matches(msg, s.keyMap.Category1) && len(s.categories) > 1:
			s.selectCategory(1)
		case key.Matches(msg, s.keyMap.Category2) && len(s.categories) > 2:
			s.selectCategory(2)
		case key.Matches(msg, s.keyMap.Category3) && len(s.categories) > 3:
			s.selectCategory(3)
		case key.Matches(msg, s.keyMap.Category4) && len(s.categories) > 4:
			s.selectCategory(4)
		case key.Matches(msg, s.keyMap.Category5) && len(s.categories) > 5:
			s.selectCategory(5)
		case key.Matches(msg, s.keyMap.Category6) && len(s.categories) > 6:
			s.selectCategory(6)
		case key.Matches(msg, s.keyMap.Category7) && len(s.categories) > 7:
			s.selectCategory(7)
		case key.Matches(msg, s.keyMap.Category8) && len(s.categories) > 8:
			s.selectCategory(8)
		case key.Matches(msg, s.keyMap.Category9) && len(s.categories) > 9:
			s.selectCategory(9)

		case key.Matches(msg, s.keyMap.Previous):
			s.list.Focus()
			if s.list.IsSelectedFirst() {
				s.list.SelectLast()
				s.list.ScrollToBottom()
				break
			}
			s.list.SelectPrev()
			s.list.ScrollToSelected()

		case key.Matches(msg, s.keyMap.Next):
			s.list.Focus()
			if s.list.IsSelectedLast() {
				s.list.SelectFirst()
				s.list.ScrollToTop()
				break
			}
			s.list.SelectNext()
			s.list.ScrollToSelected()

		case key.Matches(msg, s.keyMap.Select):
			selectedItem := s.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			skillItem, ok := selectedItem.(*SkillsLibraryItem)
			if !ok {
				break
			}
			return ActionSelectSkill{
				SkillName:        skillItem.skill.Name,
				SkillDescription: skillItem.skill.Description,
				SkillContent:     skillItem.skill.Instructions,
				SkillCategory:    skillCategory(skillItem.skill),
			}

		case key.Matches(msg, s.keyMap.FocusFilter):
			s.input.Focus()

		case key.Matches(msg, s.keyMap.Expand):
			selectedItem := s.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			if skillItem, ok := selectedItem.(*SkillsLibraryItem); ok {
				skillItem.expanded = !skillItem.expanded
				skillItem.cache = nil // Clear cache to re-render
				skillItem.Bump()
			}

		default:
			var cmd tea.Cmd
			s.input, cmd = s.input.Update(msg)
			value := s.input.Value()

			s.list.SetFilter(value)
			s.list.ScrollToTop()
			s.list.SetSelected(0)

			return ActionCmd{cmd}
		}
	}
	return nil
}

// Cursor returns cursor position.
func (s *SkillsLibrary) Cursor() *tea.Cursor {
	return InputCursor(s.com.Styles, s.input.Cursor())
}

// Draw implements Dialog.
func (s *SkillsLibrary) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := s.com.Styles
	width := max(0, min(skillsLibraryDialogMaxWidth, area.Dx()))
	height := max(0, min(skillsLibraryDialogMaxHeight, area.Dy()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	categoryBarHeight := 1
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		categoryBarHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	s.input.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
	s.list.SetSize(innerWidth, height-heightOffset)
	s.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)

	// Title with count
	totalCount := len(s.skills)
	catName := ""
	if s.selectedCategory < len(s.categories) {
		catName = s.categories[s.selectedCategory].Name
	}
	if catName == "All" || catName == "" {
		rc.Title = "Skills Library"
		rc.TitleInfo = t.Dialog.SecondaryText.Render(strings.Join([]string{"", string(rune(countDigits(totalCount))), " skills"}, ""))
	} else {
		rc.Title = "Skills Library"
		rc.TitleInfo = t.Dialog.SecondaryText.Render(" " + catName)
	}

	// Category tab bar
	categoryBar := s.categoryTabBar(innerWidth)
	rc.AddPart(categoryBar)

	// Filter input
	inputView := t.Dialog.InputPrompt.Render(s.input.View())
	rc.AddPart(inputView)

	// List
	visibleCount := len(s.list.FilteredItems())
	if visibleCount == 0 {
		emptyView := "\n\n" + t.Dialog.SecondaryText.Render("  No skills found in this category.") + "\n"
		emptyView += t.Dialog.SecondaryText.Render("  Check extensibility/skills/ to add more.")
		rc.AddPart(emptyView)
	} else {
		if s.list.Height() >= visibleCount {
			s.list.ScrollToTop()
		} else {
			s.list.ScrollToSelected()
		}
		listView := t.Dialog.List.Height(s.list.Height()).Render(s.list.Render())
		rc.AddPart(listView)
	}

	rc.Help = renderDialogHelp(t, &s.help, s, innerWidth)

	view := rc.Render()

	cur := s.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements help.KeyMap.
func (s *SkillsLibrary) ShortHelp() []key.Binding {
	return []key.Binding{
		s.keyMap.UpDown,
		s.keyMap.CategoryNext,
		s.keyMap.Select,
		s.keyMap.Close,
	}
}

// FullHelp implements help.KeyMap.
func (s *SkillsLibrary) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{s.keyMap.Select, s.keyMap.Next, s.keyMap.Previous, s.keyMap.FocusFilter},
		{s.keyMap.CategoryNext, s.keyMap.CategoryPrev, s.keyMap.Close},
	}
}

// Filter implements ListItem.
// Finished implements list.Item. Library items are render-stable
// outside of explicit SetFocused / SetMatch calls, which invalidate their
// internal render cache.
func (s *SkillsLibraryItem) Finished() bool {
	return true
}

func (s *SkillsLibraryItem) Filter() string {
	return s.skill.Name + " • " + s.skill.Description
}

// ID implements ListItem.
func (s *SkillsLibraryItem) ID() string {
	return s.skill.Name
}

// SetFocused implements ListItem.
func (s *SkillsLibraryItem) SetFocused(focused bool) {
	if s.focused == focused {
		return
	}
	s.cache = nil
	s.focused = focused
	if s.Versioned != nil {
		s.Bump()
	}
}

// SetMatch implements ListItem.
func (s *SkillsLibraryItem) SetMatch(match fuzzy.Match) {
	if sameFuzzyMatch(s.m, match) {
		return
	}
	s.cache = nil
	s.m = match
	if s.Versioned != nil {
		s.Bump()
	}
}

// Render implements ListItem.
func (s *SkillsLibraryItem) Render(width int) string {
	styles := ListItemStyles{
		ItemBlurred:     s.t.Dialog.NormalItem,
		ItemFocused:     s.t.Dialog.SelectedItem.Bold(true),
		InfoTextBlurred: s.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: s.t.Dialog.ListItem.InfoFocused,
	}

	category := skillCategory(s.skill)
	displayInfo := ""
	if category != "" {
		displayInfo = " [" + category + "]"
	}

	var metaStyle string
	if s.focused {
		// Emphasize the description in the primary color when focused.
		metaStyle = s.t.Dialog.PrimaryText.Render(" • " + s.skill.Description)
	} else {
		metaStyle = s.t.Dialog.SecondaryText.Render(" • " + s.skill.Description)
	}
	displayTitle := s.skill.Name + metaStyle

	if s.expanded {
		// Show preview of skill instructions when expanded
		preview := s.skill.Instructions
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}

		if s.focused {
			displayTitle = displayTitle + "\n\n" + s.t.Dialog.PrimaryText.Render(preview)
		} else {
			displayTitle = displayTitle + "\n\n" + s.t.Dialog.SecondaryText.Render(preview)
		}
	}

	return renderItem(styles, displayTitle, displayInfo, s.focused, width, s.cache, &s.m)
}

// skillCategory returns the category a skill declares in its frontmatter
// metadata. The chassis Skill type carries categories as metadata rather
// than a dedicated field.
func skillCategory(skill *skills.Skill) string {
	return skill.Metadata["category"]
}
