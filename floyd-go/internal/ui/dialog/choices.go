package dialog

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

type (
	Choice       struct{ Value, Label string }
	ActionChoose struct{ DialogID, Value string }
)

// Choices keeps its query and selection while a child is on the overlay stack.
type Choices struct {
	com       *common.Common
	id, title string
	items     []Choice
	input     textinput.Model
	selected  int
	help      help.Model
}

func NewChoices(com *common.Common, id, title string, items []Choice) *Choices {
	d := &Choices{com: com, id: id, title: title, items: items, input: textinput.New(), help: help.New()}
	d.input.Placeholder = "Filter"
	d.input.SetStyles(com.Styles.TextInput)
	d.input.Focus()
	d.help.Styles = com.Styles.DialogHelpStyles()
	return d
}
func (d *Choices) ID() string { return d.id }
func (d *Choices) filtered() []Choice {
	var out []Choice
	query := strings.ToLower(strings.TrimSpace(d.input.Value()))
	for _, item := range d.items {
		if strings.Contains(strings.ToLower(item.Label+" "+item.Value), query) {
			out = append(out, item)
		}
	}
	return out
}

func (d *Choices) HandleMsg(msg tea.Msg) Action {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	items := d.filtered()
	switch {
	case key.Matches(k, CloseKey):
		return ActionClose{}
	case k.String() == "up" || k.String() == "ctrl+p":
		if len(items) > 0 {
			d.selected = (d.selected + len(items) - 1) % len(items)
		}
		return nil
	case k.String() == "down" || k.String() == "ctrl+n":
		if len(items) > 0 {
			d.selected = (d.selected + 1) % len(items)
		}
		return nil
	case k.String() == "enter":
		if len(items) == 0 {
			return ActionCmd{util.ReportWarn("No matches. Change the filter or go back.")}
		}
		return ActionChoose{DialogID: d.id, Value: items[d.selected].Value}
	default:
		before := d.input.Value()
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(k)
		if before != d.input.Value() {
			d.selected = 0
		}
		return ActionCmd{cmd}
	}
}

func (d *Choices) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	inner := max(0, width-t.Dialog.View.GetHorizontalFrameSize())
	d.input.SetWidth(max(1, inner-2))
	rc := NewRenderContext(t, width)
	rc.Title = d.title
	rc.AddPart(t.Dialog.InputPrompt.Render(d.input.View()))
	items := d.filtered()
	n := max(1, min(12, area.Dy()-10))
	start := max(0, d.selected-n+1)
	var lines []string
	for i := start; i < min(len(items), start+n); i++ {
		prefix := "  "
		if i == d.selected {
			prefix = "> "
		}
		lines = append(lines, t.Dialog.PrimaryText.Width(inner).Render(prefix+items[i].Label))
	}
	if len(items) == 0 {
		lines = []string{"No matches"}
	}
	rc.AddPart(strings.Join(lines, "\n"))
	rc.Help = renderDialogHelp(t, &d.help, d, inner)
	DrawCenter(scr, area, rc.Render())
	return nil
}

func (d *Choices) ShortHelp() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "choose")), key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")), CloseKey}
}
func (d *Choices) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }
