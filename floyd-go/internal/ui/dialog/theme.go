package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/sahilm/fuzzy"
)

const (
	// ThemeID is the identifier for the theme picker dialog.
	ThemeID              = "theme"
	themeDialogMaxWidth  = 46
	themeDialogMinHeight = 8
	themeDialogMaxHeight = 14
)

// Theme is a picker for the built-in TUI palettes.
type Theme struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	keyMap struct {
		Select   key.Binding
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Close    key.Binding
	}
}

// ThemeItem is one selectable palette.
type ThemeItem struct {
	*list.Versioned
	name      string
	title     string
	desc      string
	isCurrent bool
	t         *styles.Styles
	m         fuzzy.Match
	cache     map[int]string
	focused   bool
}

var (
	_ Dialog   = (*Theme)(nil)
	_ ListItem = (*ThemeItem)(nil)
)

// NewTheme creates the theme picker, preselecting current.
func NewTheme(com *common.Common, current string) *Theme {
	d := &Theme{com: com}

	d.help = help.New()
	d.help.Styles = com.Styles.DialogHelpStyles()

	d.list = list.NewFilterableList()
	d.list.Focus()

	d.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "confirm"),
	)
	d.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next item"),
	)
	d.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	d.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	d.keyMap.Close = CloseKey

	items := make([]list.FilterableItem, 0, len(styles.ThemeNames()))
	selected := 0
	for i, info := range styles.ThemeNames() {
		items = append(items, &ThemeItem{
			Versioned: list.NewVersioned(),
			name:      info.Name,
			title:     info.Name,
			desc:      info.Description,
			isCurrent: info.Name == current,
			t:         com.Styles,
		})
		if info.Name == current {
			selected = i
		}
	}
	d.list.SetItems(items...)
	d.list.SetSelected(selected)
	d.list.ScrollToSelected()
	return d
}

// ID implements Dialog.
func (d *Theme) ID() string {
	return ThemeID
}

// HandleMsg implements [Dialog].
func (d *Theme) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, d.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, d.keyMap.Previous):
		d.list.Focus()
		if d.list.IsSelectedFirst() {
			d.list.SelectLast()
			d.list.ScrollToBottom()
			break
		}
		d.list.SelectPrev()
		d.list.ScrollToSelected()
	case key.Matches(keyMsg, d.keyMap.Next):
		d.list.Focus()
		if d.list.IsSelectedLast() {
			d.list.SelectFirst()
			d.list.ScrollToTop()
			break
		}
		d.list.SelectNext()
		d.list.ScrollToSelected()
	case key.Matches(keyMsg, d.keyMap.Select):
		item, ok := d.list.SelectedItem().(*ThemeItem)
		if !ok {
			break
		}
		return ActionSelectTheme{Name: item.name}
	}
	return nil
}

// Draw implements [Dialog].
func (d *Theme) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(themeDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	listTotalHeight := d.list.TotalHeight()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() + t.Dialog.View.GetVerticalFrameSize()
	desiredHeight := heightOffset + listTotalHeight
	maxAvailable := area.Dy() - t.Dialog.View.GetVerticalBorderSize()
	height := max(themeDialogMinHeight, min(themeDialogMaxHeight, desiredHeight, maxAvailable))

	listHeight, listTotalHeight, _ := sizeDialogList(t, d.list, innerWidth, height)

	rc := NewRenderContext(t, width)
	rc.Title = "Select Theme"
	listView := t.Dialog.List.Height(d.list.Height()).Render(d.list.Render())
	listView = joinScrollbar(t, listView, listHeight, listTotalHeight, listHeight, d.list.Offset())
	rc.AddPart(listView)
	rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)

	DrawCenter(scr, area, rc.Render())
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *Theme) ShortHelp() []key.Binding {
	return []key.Binding{
		d.keyMap.UpDown,
		d.keyMap.Select,
		d.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (d *Theme) FullHelp() [][]key.Binding {
	return [][]key.Binding{{
		d.keyMap.Select,
		d.keyMap.Next,
		d.keyMap.Previous,
		d.keyMap.Close,
	}}
}

// Filter implements list.FilterableItem.
func (i *ThemeItem) Filter() string { return i.title }

// Finished implements list.Item. Theme rows are render-stable outside
// explicit SetFocused / SetMatch calls.
func (i *ThemeItem) Finished() bool { return true }

// ID implements list.FilterableItem.
func (i *ThemeItem) ID() string { return i.name }

// SetFocused implements list.FilterableItem.
func (i *ThemeItem) SetFocused(focused bool) {
	if i.focused == focused {
		return
	}
	i.cache = nil
	i.focused = focused
	if i.Versioned != nil {
		i.Bump()
	}
}

// SetMatch implements list.FilterableItem.
func (i *ThemeItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(i.m, m) {
		return
	}
	i.cache = nil
	i.m = m
	if i.Versioned != nil {
		i.Bump()
	}
}

// Render implements list.Item.
func (i *ThemeItem) Render(width int) string {
	info := i.desc
	if i.isCurrent {
		info = "current"
	}
	styles := ListItemStyles{
		ItemBlurred:     i.t.Dialog.NormalItem,
		ItemFocused:     i.t.Dialog.SelectedItem,
		InfoTextBlurred: i.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: i.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(styles, i.title, info, i.focused, width, i.cache, &i.m)
}
