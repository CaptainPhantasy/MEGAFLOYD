package dialog

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/list"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// CommandsID is the identifier for the commands dialog.
const CommandsID = "commands"

// CommandType represents the type of commands being displayed.
type CommandType uint

// String returns the string representation of the CommandType.
func (c CommandType) String() string { return []string{"System", "User", "MCP"}[c] }

const (
	sidebarCompactModeBreakpoint = 120
)

const (
	SystemCommands CommandType = iota
	UserCommands
	MCPPrompts
)

// Commands represents a dialog that shows available commands.
type dockerMCPAvailabilityCheckedMsg struct {
	available bool
}

type Commands struct {
	com    *common.Common
	keyMap struct {
		Select,
		UpDown,
		Next,
		Previous,
		Tab,
		ShiftTab,
		Close key.Binding
	}

	sessionID  string
	hasSession bool
	hasTodos   bool
	hasQueue   bool
	selected   CommandType

	spinner spinner.Model
	loading bool

	help  help.Model
	input textinput.Model
	list  *list.FilterableList

	windowWidth int

	customCommands []commands.CustomCommand
	mcpPrompts     []commands.MCPPrompt

	dockerMCPAvailable     *bool
	dockerMCPCheckInFlight bool
}

var _ Dialog = (*Commands)(nil)

// NewCommands creates a new commands dialog.
func NewCommands(com *common.Common, sessionID string, hasSession, hasTodos, hasQueue bool, customCommands []commands.CustomCommand, mcpPrompts []commands.MCPPrompt) (*Commands, error) {
	c := &Commands{
		com:            com,
		selected:       SystemCommands,
		sessionID:      sessionID,
		hasSession:     hasSession,
		hasTodos:       hasTodos,
		hasQueue:       hasQueue,
		customCommands: customCommands,
		mcpPrompts:     mcpPrompts,
	}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()

	c.help = help

	c.list = list.NewFilterableList()
	c.list.Focus()
	c.list.SetSelected(0)

	c.input = textinput.New()
	c.input.SetVirtualCursor(false)
	c.input.Placeholder = "Type to filter"
	c.input.SetStyles(com.Styles.TextInput)
	c.input.Focus()

	c.keyMap.Select = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
	c.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	c.keyMap.Next = key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next item"),
	)
	c.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	c.keyMap.Tab = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next category"),
	)
	c.keyMap.ShiftTab = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "previous category"),
	)
	closeKey := CloseKey
	closeKey.SetHelp("esc", "cancel")
	c.keyMap.Close = closeKey

	if available, known := config.DockerMCPAvailabilityCached(); known {
		c.dockerMCPAvailable = &available
	}

	// Set initial commands
	c.setCommandItems(c.selected)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = com.Styles.Dialog.Spinner
	c.spinner = s

	return c, nil
}

// ID implements Dialog.
func (c *Commands) ID() string {
	return CommandsID
}

// HandleMsg implements [Dialog].
func (c *Commands) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case dockerMCPAvailabilityCheckedMsg:
		c.dockerMCPAvailable = &msg.available
		c.dockerMCPCheckInFlight = false
		if c.selected == SystemCommands {
			c.refreshCommandItems()
		}
		return nil
	case spinner.TickMsg:
		if c.loading {
			var cmd tea.Cmd
			c.spinner, cmd = c.spinner.Update(msg)
			return ActionCmd{Cmd: cmd}
		}
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, c.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, c.keyMap.Previous):
			c.list.Focus()
			if c.list.IsSelectedFirst() {
				c.list.SelectLast()
			} else {
				c.list.SelectPrev()
			}
			c.list.ScrollToSelected()
		case key.Matches(msg, c.keyMap.Next):
			c.list.Focus()
			if c.list.IsSelectedLast() {
				c.list.SelectFirst()
			} else {
				c.list.SelectNext()
			}
			c.list.ScrollToSelected()
		case key.Matches(msg, c.keyMap.Select):
			// Slash input typed into the filter carries its arguments with
			// it. Dispatch the whole line so "theme light" and
			// "plan status" reach their handlers instead of being treated
			// as filter text that matches nothing. The leading slash is
			// optional because opening the palette consumes it.
			if typed := strings.TrimSpace(c.input.Value()); typed != "" {
				fields := strings.Fields(typed)
				name := strings.TrimPrefix(fields[0], "/")
				if name == "" {
					break
				}
				spec, known := commands.Builtin(strings.ToLower(name))
				slashPrefixed := strings.HasPrefix(typed, "/")
				// An exact command name is an invocation. Fuzzy matches in
				// another row's argument hint must not redirect it (for
				// example, "provider" also matches /logout [provider]).
				if known && len(fields) == 1 {
					return ActionRunBuiltin{Name: spec.Name, FromPalette: true}
				}
				if known || slashPrefixed {
					input := typed
					if !slashPrefixed {
						input = "/" + typed
					}
					return ActionSlashInput{Input: input}
				}
			}
			if selectedItem := c.list.SelectedItem(); selectedItem != nil {
				if item, ok := selectedItem.(*CommandItem); ok && item != nil {
					return item.Action()
				}
			}
			return ActionCmd{Cmd: util.ReportWarn("No matching command. Change the filter or press Esc to go back.")}
		case key.Matches(msg, c.keyMap.Tab):
			if len(c.customCommands) > 0 || len(c.mcpPrompts) > 0 {
				c.selected = c.nextCommandType()
				c.setCommandItems(c.selected)
			}
		case key.Matches(msg, c.keyMap.ShiftTab):
			if len(c.customCommands) > 0 || len(c.mcpPrompts) > 0 {
				c.selected = c.previousCommandType()
				c.setCommandItems(c.selected)
			}
		default:
			var cmd tea.Cmd
			for _, item := range c.list.FilteredItems() {
				if item, ok := item.(*CommandItem); ok && item != nil {
					if msg.String() == item.Shortcut() {
						return item.Action()
					}
				}
			}
			prevValue := c.input.Value()
			c.input, cmd = c.input.Update(msg)
			value := c.input.Value()
			if value != prevValue {
				// A leading slash is optional in the filter so a pasted
				// "/usage" matches the "/usage" entry.
				c.list.SetFilter(strings.TrimPrefix(value, "/"))
				c.list.ScrollToTop()
				c.list.SetSelected(0)
			}
			return ActionCmd{cmd}
		}
	}
	return nil
}

func checkDockerMCPAvailabilityCmd() tea.Cmd {
	return func() tea.Msg {
		return dockerMCPAvailabilityCheckedMsg{available: config.RefreshDockerMCPAvailability()}
	}
}

func (c *Commands) InitialCmd() tea.Cmd {
	if c.dockerMCPAvailable != nil || c.dockerMCPCheckInFlight {
		return nil
	}
	c.dockerMCPCheckInFlight = true
	return checkDockerMCPAvailabilityCmd()
}

// Cursor returns the cursor position relative to the dialog.
func (c *Commands) Cursor() *tea.Cursor {
	return InputCursor(c.com.Styles, c.input.Cursor())
}

// commandsRadioView generates the command type selector radio buttons.
func commandsRadioView(sty *styles.Styles, selected CommandType, hasUserCmds bool, hasMCPPrompts bool) string {
	if !hasUserCmds && !hasMCPPrompts {
		return ""
	}

	selectedFn := func(t CommandType) string {
		if t == selected {
			return sty.Radio.On.Padding(0, 1).Render() + sty.Radio.Label.Render(t.String())
		}
		return sty.Radio.Off.Padding(0, 1).Render() + sty.Radio.Label.Render(t.String())
	}

	parts := []string{
		selectedFn(SystemCommands),
	}

	if hasUserCmds {
		parts = append(parts, selectedFn(UserCommands))
	}
	if hasMCPPrompts {
		parts = append(parts, selectedFn(MCPPrompts))
	}

	return strings.Join(parts, " ")
}

// Draw implements [Dialog].
func (c *Commands) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := c.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	if area.Dx() != c.windowWidth && c.selected == SystemCommands {
		c.windowWidth = area.Dx()
		// since some items in the list depend on width (e.g. toggle sidebar command),
		// we need to reset the command items when width changes
		c.setCommandItems(c.selected)
	}

	innerWidth := width - c.com.Styles.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	c.input.SetWidth(dialogInputTextWidth(t, c.input, innerWidth))

	c.list.SetSize(innerWidth, max(0, height-heightOffset))

	// Hide the shortcut hints uniformly when the widest would crowd names.
	applyInfoColumnVisibility(c.list.FilteredItems(), innerWidth, commandInfoMaxPercent)

	rc := NewRenderContext(t, width)
	rc.Title = "Commands"
	rc.TitleInfo = commandsRadioView(t, c.selected, len(c.customCommands) > 0, len(c.mcpPrompts) > 0)
	inputView := t.Dialog.InputPrompt.Render(c.input.View())
	rc.AddPart(inputView)
	listView := t.Dialog.List.Height(c.list.Height()).Render(c.list.Render())
	rc.AddPart(listView)
	rc.Help = renderDialogHelp(t, &c.help, c, innerWidth)

	if c.loading {
		rc.Help = t.Dialog.HelpView.Width(innerWidth).Render(c.spinner.View() + " Generating Prompt...")
	}

	view := rc.Render()

	cur := c.Cursor()
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (c *Commands) ShortHelp() []key.Binding {
	bindings := []key.Binding{
		c.keyMap.UpDown,
		c.keyMap.Select,
		c.keyMap.Close,
	}
	if len(c.customCommands) > 0 || len(c.mcpPrompts) > 0 {
		bindings = append([]key.Binding{c.keyMap.Tab, c.keyMap.ShiftTab}, bindings...)
	}
	return bindings
}

// FullHelp implements [help.KeyMap].
func (c *Commands) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		c.ShortHelp(),
	}
}

// nextCommandType returns the next command type in the cycle.
func (c *Commands) nextCommandType() CommandType {
	switch c.selected {
	case SystemCommands:
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		fallthrough
	case UserCommands:
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		fallthrough
	case MCPPrompts:
		return SystemCommands
	default:
		return SystemCommands
	}
}

// previousCommandType returns the previous command type in the cycle.
func (c *Commands) previousCommandType() CommandType {
	switch c.selected {
	case SystemCommands:
		if len(c.mcpPrompts) > 0 {
			return MCPPrompts
		}
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		return SystemCommands
	case UserCommands:
		return SystemCommands
	case MCPPrompts:
		if len(c.customCommands) > 0 {
			return UserCommands
		}
		return SystemCommands
	default:
		return SystemCommands
	}
}

// setCommandItems sets the command items based on the specified command type.
func (c *Commands) setCommandItems(commandType CommandType) {
	c.selected = commandType

	commandItems := []list.FilterableItem{}
	switch c.selected {
	case SystemCommands:
		for _, cmd := range c.defaultCommands() {
			commandItems = append(commandItems, cmd)
		}
	case UserCommands:
		for _, cmd := range c.customCommands {
			var action Action
			if cmd.Skill != nil {
				action = ActionAttachSkill{ID: cmd.Skill.SkillFilePath, Name: cmd.Skill.Name}
			} else {
				action = ActionRunCustomCommand{
					Content:   cmd.Content,
					Arguments: cmd.Arguments,
					Skill:     cmd.Skill,
				}
			}
			item := NewCommandItem(c.com.Styles, "custom_"+cmd.ID, cmd.Name, "", action)
			if cmd.Skill != nil {
				item = item.WithDescription(cmd.Skill.Description)
			}
			commandItems = append(commandItems, item)
		}
	case MCPPrompts:
		for _, cmd := range c.mcpPrompts {
			action := ActionRunMCPPrompt{
				Title:       cmd.Title,
				Description: cmd.Description,
				PromptID:    cmd.PromptID,
				ClientID:    cmd.ClientID,
				Arguments:   cmd.Arguments,
			}
			commandItems = append(commandItems, NewCommandItem(c.com.Styles, "mcp_"+cmd.ID, cmd.PromptID, "", action))
		}
	}

	c.list.SetItems(commandItems...)
	c.list.SetFilter("")
	c.list.ScrollToTop()
	c.list.SetSelected(0)
	c.input.SetValue("")
}

// refreshCommandItems retains input and selection when background discovery
// updates a category. Only an explicit category change resets the filter.
func (c *Commands) refreshCommandItems() {
	query := c.input.Value()
	var selectedID string
	if item, ok := c.list.SelectedItem().(*CommandItem); ok && item != nil {
		selectedID = item.id
	}
	c.setCommandItems(c.selected)
	c.input.SetValue(query)
	c.list.SetFilter(strings.TrimPrefix(query, "/"))
	for i, item := range c.list.FilteredItems() {
		if command, ok := item.(*CommandItem); ok && command.id == selectedID {
			c.list.SetSelected(i)
			c.list.ScrollToSelected()
			break
		}
	}
}

// builtinShortcut returns the keybinding hint for a builtin command, or
// the empty string when the command has no direct keybinding.
func builtinShortcut(name string) string {
	switch name {
	case "new":
		return "ctrl+n"
	case "sessions":
		return "ctrl+s"
	case "model":
		return "ctrl+l"
	case "yolo":
		return "ctrl+y"
	case "editor":
		return "ctrl+o"
	case "help":
		return "ctrl+g"
	case "exit":
		return "ctrl+c"
	default:
		return ""
	}
}

// builtinPaletteItems builds one palette entry per registered slash
// command. Selection and hand-typed input share ActionRunBuiltin.
func builtinPaletteItems(t *styles.Styles) []*CommandItem {
	specs := commands.Builtins()
	items := make([]*CommandItem, 0, len(specs))
	for _, spec := range specs {
		item := NewCommandItem(
			t,
			"cmd_"+spec.Name,
			"/"+spec.Name,
			builtinShortcut(spec.Name),
			ActionRunBuiltin{Name: spec.Name, FromPalette: true},
		).WithDescription(spec.Description).WithArgumentHint(spec.ArgumentHint)
		if len(spec.Aliases) > 0 {
			item = item.WithAliases(spec.Aliases...)
		}
		items = append(items, item)
	}
	return items
}

// defaultCommands returns the list of default system commands.
func (c *Commands) defaultCommands() []*CommandItem {
	items := make([]*CommandItem, 0, len(commands.Builtins())+16)
	items = append(items, builtinPaletteItems(c.com.Styles)...)

	items = append(
		items,
		NewCommandItem(c.com.Styles, "agent_library", "Agent Library", "", ActionOpenDialog{AgentLibraryID}),
		NewCommandItem(c.com.Styles, "skills_library", "Skills Library", "", ActionOpenDialog{SkillsLibraryID}),
	)

	cfg := c.com.Config()

	// Only show toggle compact mode command if window width is larger than compact breakpoint (120)
	if c.windowWidth >= sidebarCompactModeBreakpoint && c.hasSession {
		items = append(items, NewCommandItem(c.com.Styles, "toggle_sidebar", "Toggle Sidebar", "", ActionToggleCompactMode{}))
	}
	if c.hasSession {
		agentCfg := cfg.Agents[config.AgentCoder]
		model := cfg.GetModelByType(agentCfg.Model)
		if model != nil && model.SupportsImages {
			items = append(items, NewCommandItem(c.com.Styles, "file_picker", "Open File Picker", "ctrl+f", ActionOpenDialog{
				DialogID: FilePickerID,
			}))
		}
	}

	// Add Docker MCP command if available and not already enabled.
	if !cfg.IsDockerMCPEnabled() && c.dockerMCPAvailable != nil && *c.dockerMCPAvailable {
		items = append(items, NewCommandItem(c.com.Styles, "enable_docker_mcp", "Enable Docker MCP Catalog", "", ActionEnableDockerMCP{}))
	}

	// Add disable Docker MCP command if it's currently enabled
	if cfg.IsDockerMCPEnabled() {
		items = append(items, NewCommandItem(c.com.Styles, "disable_docker_mcp", "Disable Docker MCP Catalog", "", ActionDisableDockerMCP{}))
	}

	if c.hasTodos || c.hasQueue {
		var label string
		switch {
		case c.hasTodos && c.hasQueue:
			label = "Toggle To-Dos/Queue"
		case c.hasQueue:
			label = "Toggle Queue"
		default:
			label = "Toggle To-Dos"
		}
		items = append(items, NewCommandItem(c.com.Styles, "toggle_pills", label, "ctrl+t", ActionTogglePills{}))
	}

	// Add a command for selecting notification style via picker dialog.
	notificationLabel := "Notification Style"
	items = append(items, NewCommandItem(c.com.Styles, "select_notifications", notificationLabel, "", ActionOpenDialog{DialogID: NotificationsID}))

	// Add transparent background toggle.
	transparentLabel := "Disable Background Color"
	if cfg != nil && cfg.Options != nil && cfg.Options.TUI.IsTransparent() {
		transparentLabel = "Enable Background Color"
	}
	items = append(items, NewCommandItem(c.com.Styles, "toggle_transparent", transparentLabel, "", ActionToggleTransparentBackground{}))
	items = append(items, NewCommandItem(c.com.Styles, "toggle_terminal", "Toggle Terminal", "alt+t", ActionToggleTerminal{}))

	return items
}

// SetCustomCommands sets the custom commands and refreshes the view if user commands are currently displayed.
func (c *Commands) SetCustomCommands(customCommands []commands.CustomCommand) {
	c.customCommands = customCommands
	if c.selected == UserCommands {
		c.refreshCommandItems()
	}
}

// SetMCPPrompts sets the MCP prompts and refreshes the view if MCP prompts are currently displayed.
func (c *Commands) SetMCPPrompts(mcpPrompts []commands.MCPPrompt) {
	c.mcpPrompts = mcpPrompts
	if c.selected == MCPPrompts {
		c.refreshCommandItems()
	}
}

// StartLoading implements [LoadingDialog].
func (c *Commands) StartLoading() tea.Cmd {
	if c.loading {
		return nil
	}
	c.loading = true
	return c.spinner.Tick
}

// StopLoading implements [LoadingDialog].
func (c *Commands) StopLoading() {
	c.loading = false
}
