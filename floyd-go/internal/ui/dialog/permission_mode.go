package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/ui/common"
)

// PermissionModeID is the identifier for the permission mode picker.
const PermissionModeID = "permission_mode"

type permissionModeOption struct {
	label        string
	description  string
	skipRequests bool
}

var permissionModeOptions = []permissionModeOption{
	{
		label:        "Ask When Needed",
		description:  "Routine edits ask for permission before running",
		skipRequests: false,
	},
	{
		label:        "Never Ask",
		description:  "Every tool call runs without a permission prompt",
		skipRequests: true,
	},
}

// PermissionMode is the permission mode picker opened by /permission,
// /yolo and /auto. It maps the two modes the chassis supports onto the
// workspace's skip-requests flag.
type PermissionMode struct {
	com      *common.Common
	help     help.Model
	selected int

	keyMap struct {
		Up,
		Down,
		Select,
		Close key.Binding
	}
}

var _ Dialog = (*PermissionMode)(nil)

// NewPermissionMode creates the picker with the current mode selected.
func NewPermissionMode(com *common.Common, skipRequests bool) *PermissionMode {
	m := &PermissionMode{com: com, selected: 0}
	if skipRequests {
		m.selected = 1
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	m.help = h

	m.keyMap.Up = key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/↓", "choose"),
	)
	m.keyMap.Down = key.NewBinding(
		key.WithKeys("down", "j"),
	)
	m.keyMap.Select = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
	m.keyMap.Close = CloseKey

	return m
}

// ID implements Dialog.
func (m *PermissionMode) ID() string { return PermissionModeID }

// HandleMsg implements Dialog.
func (m *PermissionMode) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	switch {
	case key.Matches(keyMsg, m.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, m.keyMap.Up):
		m.selected = (m.selected + len(permissionModeOptions) - 1) % len(permissionModeOptions)
	case key.Matches(keyMsg, m.keyMap.Down):
		m.selected = (m.selected + 1) % len(permissionModeOptions)
	case key.Matches(keyMsg, m.keyMap.Select):
		return ActionSetPermissionMode{
			SkipRequests: permissionModeOptions[m.selected].skipRequests,
		}
	}
	return nil
}

// Cursor implements Dialog.
func (m *PermissionMode) Cursor() *tea.Cursor { return nil }

// Draw implements Dialog.
func (m *PermissionMode) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	rc := NewRenderContext(t, width)
	rc.Title = "Permission Mode"

	for i, opt := range permissionModeOptions {
		radio := t.Radio.Off
		label := t.Dialog.NormalItem
		if i == m.selected {
			radio = t.Radio.On
			label = t.Dialog.SelectedItem
		}
		rc.AddPart(radio.Padding(0, 1).Render() + label.Render(opt.label))
		rc.AddPart(t.Dialog.SecondaryText.PaddingLeft(4).Render(opt.description))
	}

	rc.Help = renderDialogHelp(t, &m.help, m, innerWidth)
	DrawCenterCursor(scr, area, rc.Render(), nil)
	return nil
}

// ShortHelp implements help.KeyMap.
func (m *PermissionMode) ShortHelp() []key.Binding {
	return []key.Binding{m.keyMap.Up, m.keyMap.Select, m.keyMap.Close}
}

// FullHelp implements help.KeyMap.
func (m *PermissionMode) FullHelp() [][]key.Binding {
	return [][]key.Binding{{m.keyMap.Up, m.keyMap.Down, m.keyMap.Select, m.keyMap.Close}}
}
