package model

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/agent/ralph"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/clipboard"
	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/shell"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/version"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/pkg/browser"
)

// feedbackURL is the documented issue tracker for this fork (see root.go).
const feedbackURL = "https://github.com/legacy-ai/floyd/issues/new?template=bug.yml"

// builtinHandlerID identifies the concrete handler behind a registered
// slash command. Tests enumerate commands.Builtins and assert every name
// maps to a known handler, so no command can become a dead entry.
type builtinHandlerID uint8

const (
	handlerUnknown builtinHandlerID = iota
	handlerYolo
	handlerAuto
	handlerPermission
	handlerSettings
	handlerModel
	handlerSecondaryModel
	handlerEffort
	handlerProvider
	handlerHelp
	handlerNew
	handlerSessions
	handlerTasks
	handlerMCP
	handlerPlugins
	handlerCompact
	handlerGoal
	handlerInit
	handlerFork
	handlerTitle
	handlerUsage
	handlerStatus
	handlerUndo
	handlerEditor
	handlerLogin
	handlerLogout
	handlerExportMarkdown
	handlerExportDebugZip
	handlerCopy
	handlerExit
	handlerVersion
	handlerSwarm
	handlerBtw
	handlerFeedback
	handlerReload
	handlerReloadTUI
)

// builtinHandlerFor maps a canonical command name to its handler.
func builtinHandlerFor(name string) builtinHandlerID {
	switch name {
	case "yolo":
		return handlerYolo
	case "auto":
		return handlerAuto
	case "permission":
		return handlerPermission
	case "settings":
		return handlerSettings
	case "model":
		return handlerModel
	case "secondary-model":
		return handlerSecondaryModel
	case "effort":
		return handlerEffort
	case "provider":
		return handlerProvider
	case "help":
		return handlerHelp
	case "new":
		return handlerNew
	case "sessions":
		return handlerSessions
	case "tasks":
		return handlerTasks
	case "mcp":
		return handlerMCP
	case "plugins":
		return handlerPlugins
	case "compact":
		return handlerCompact
	case "goal":
		return handlerGoal
	case "init":
		return handlerInit
	case "fork":
		return handlerFork
	case "title":
		return handlerTitle
	case "usage":
		return handlerUsage
	case "status":
		return handlerStatus
	case "undo":
		return handlerUndo
	case "editor":
		return handlerEditor
	case "login":
		return handlerLogin
	case "logout":
		return handlerLogout
	case "export-md":
		return handlerExportMarkdown
	case "export-debug-zip":
		return handlerExportDebugZip
	case "copy":
		return handlerCopy
	case "exit":
		return handlerExit
	case "version":
		return handlerVersion
	case "swarm":
		return handlerSwarm
	case "btw":
		return handlerBtw
	case "feedback":
		return handlerFeedback
	case "reload":
		return handlerReload
	case "reload-tui":
		return handlerReloadTUI
	default:
		return handlerUnknown
	}
}

// builtinNeedsArgs reports commands that only do something useful with an
// argument. Selecting one from the palette pre-fills the slash prefix.
func builtinNeedsArgs(name string) bool {
	switch name {
	case "btw", "goal", "swarm":
		return true
	default:
		return false
	}
}

// builtinResultMsg carries the outcome of an asynchronous builtin handler
// back to the Update loop, which owns all model mutation.
type builtinResultMsg struct {
	Title     string
	Body      string
	Info      string
	Err       error
	Clipboard string
	Send      string
	ReloadTUI bool
}

// parseSlashInput splits a leading-slash input into a command name and
// argument string. It reports false when the input is not a slash command.
func parseSlashInput(input string) (name, args string, ok bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return "", "", false
	}
	body := strings.TrimPrefix(trimmed, "/")
	name, args, _ = strings.Cut(body, " ")
	name = strings.ToLower(strings.TrimSpace(name))
	args = strings.TrimSpace(args)
	if name == "" {
		return "", "", false
	}
	return name, args, true
}

// dispatchSlashInput executes a typed slash command. It reports false when
// the input is not a known builtin, so callers can send it as a normal
// prompt (matching the reference harness).
func (m *UI) dispatchSlashInput(input string) (tea.Cmd, bool) {
	name, args, ok := parseSlashInput(input)
	if !ok {
		return nil, false
	}
	spec, found := commands.Builtin(name)
	if !found {
		if reason, blocked := commands.BlockedBuiltin[name]; blocked {
			return util.ReportWarn("/" + name + " is not available: " + reason), true
		}
		if commands.IsExcluded(name) {
			return util.ReportWarn("/" + name + " is owned by the remote lane and is not part of this command set."), true
		}
		return nil, false
	}
	return m.runBuiltin(spec.Name, args, false), true
}

// runBuiltin executes a registered command by name.
func (m *UI) runBuiltin(name, args string, fromPalette bool) tea.Cmd {
	spec, ok := commands.Builtin(name)
	if !ok {
		return nil
	}

	if fromPalette && args == "" && builtinNeedsArgs(spec.Name) {
		return m.promptBuiltinInput(spec.Name)
	}

	switch builtinHandlerFor(spec.Name) {
	case handlerYolo:
		yolo := m.toggleYoloMode()
		status := "off"
		if yolo {
			status = "on"
		}
		return util.ReportInfo("YOLO mode " + status)

	case handlerAuto:
		return m.setSkipRequests(true)

	case handlerPermission:
		m.dialog.OpenDialog(dialog.NewPermissionMode(m.com, m.yoloModeCached()))
		return nil

	case handlerSettings:
		return m.openDialog(dialog.ConfigAuditID)

	case handlerModel:
		return m.openDialog(dialog.ModelsID)

	case handlerSecondaryModel:
		return m.openModelsDialogForType(dialog.ModelTypeSmall)

	case handlerEffort:
		return m.openEffortPicker()

	case handlerProvider:
		return m.openDialog(dialog.ModelsID)

	case handlerHelp:
		m.status.ToggleHelp()
		m.updateLayoutAndSize()
		return nil

	case handlerNew:
		if m.isAgentBusy() {
			return util.ReportWarn("Agent is busy, please wait before starting a new session...")
		}
		if cmd := m.newSession(); cmd != nil {
			return cmd
		}
		return nil

	case handlerSessions:
		return m.openDialog(dialog.SessionsID)

	case handlerTasks:
		return m.showInfo("Background Tasks", m.tasksReport())

	case handlerMCP:
		return m.openDialog(dialog.MCPServersID)

	case handlerPlugins:
		return m.openDialog(dialog.PluginsLibraryID)

	case handlerCompact:
		if m.isAgentBusy() {
			return util.ReportWarn("Agent is busy, please wait before summarizing session...")
		}
		if m.session == nil {
			return util.ReportWarn("No active session to compact.")
		}
		sessionID := m.session.ID
		ws := m.com.Workspace
		return func() tea.Msg {
			if err := ws.AgentSummarize(context.Background(), sessionID); err != nil {
				return builtinResultMsg{Err: err}
			}
			return builtinResultMsg{Info: "Session compacted."}
		}

	case handlerGoal:
		return m.goalCommand(args)

	case handlerInit:
		if m.isAgentBusy() {
			return util.ReportWarn("Agent is busy, please wait before initializing...")
		}
		return m.initializeProject()

	case handlerFork:
		return m.forkSession()

	case handlerTitle:
		return m.titleCommand(args)

	case handlerUsage:
		return m.showInfo("Usage", m.usageReport())

	case handlerStatus:
		return m.showInfo("Status", m.statusReport())

	case handlerUndo:
		return m.undoPrompts(args)

	case handlerEditor:
		return m.editorCommand(args)

	case handlerLogin:
		return m.loginCommand(args)

	case handlerLogout:
		return m.logoutProvider(args)

	case handlerExportMarkdown:
		return m.exportMarkdown(args)

	case handlerExportDebugZip:
		return m.exportDebugZip()

	case handlerCopy:
		return m.copyLastAssistant()

	case handlerExit:
		return m.openQuitDialog()

	case handlerVersion:
		return m.showInfo("Version", fmt.Sprintf(
			"Floyd %s\nCommit: %s\nBuild: %s",
			version.Version, version.Commit, version.BuildID,
		))

	case handlerSwarm:
		return m.runSwarm(args)

	case handlerBtw:
		return m.runBtw(args)

	case handlerFeedback:
		return func() tea.Msg {
			if err := browser.OpenURL(feedbackURL); err != nil {
				return builtinResultMsg{Err: fmt.Errorf("failed to open %s: %w", feedbackURL, err)}
			}
			return builtinResultMsg{Info: "Opened the issue tracker: " + feedbackURL}
		}

	case handlerReload:
		return m.reloadConfig()

	case handlerReloadTUI:
		return func() tea.Msg {
			return builtinResultMsg{Info: "UI styles reapplied.", ReloadTUI: true}
		}

	default:
		return nil
	}
}

// promptBuiltinInput places a slash prefix in the editor so the user can
// type the argument. Used by the palette for /goal, /btw and /swarm.
func (m *UI) promptBuiltinInput(name string) tea.Cmd {
	m.textarea.SetValue("/" + name + " ")
	m.textarea.MoveToEnd()
	m.textarea.Focus()
	m.setState(m.state, uiFocusEditor)
	return util.ReportInfo("Type the " + name + " argument and press enter.")
}

// openInfo opens the read-only info panel.
func (m *UI) showInfo(title, body string) tea.Cmd {
	if m.dialog.ContainsDialog(dialog.InfoID) {
		m.dialog.CloseDialog(dialog.InfoID)
	}
	m.dialog.OpenDialog(dialog.NewInfo(m.com, title, body))
	return nil
}

// loginCommand authenticates a provider directly when one is named, and
// otherwise opens the provider/model picker where selecting a model
// starts the authentication flow.
func (m *UI) loginCommand(args string) tea.Cmd {
	providerID := strings.ToLower(strings.TrimSpace(args))
	if providerID == "" {
		return tea.Batch(
			m.openDialog(dialog.ModelsID),
			util.ReportInfo("Select a provider's model to authenticate it, or run /login <provider>."),
		)
	}
	switch providerID {
	case "github", "github-copilot":
		providerID = "copilot"
	}

	cfg := m.com.Config()
	if cfg == nil {
		return util.ReportError(fmt.Errorf("configuration not found"))
	}
	pc, ok := cfg.Providers.Get(providerID)
	if !ok {
		return util.ReportWarn("Unknown provider: " + providerID)
	}
	selected := cfg.Models[config.SelectedModelTypeLarge]
	if selected.Provider != providerID || selected.Model == "" {
		selected = config.SelectedModel{Provider: providerID}
		if len(pc.Models) > 0 {
			selected.Model = pc.Models[0].ID
		}
	}
	if selected.Model == "" {
		return util.ReportWarn("Provider " + providerID + " has no configured models to authenticate.")
	}

	provider := catwalk.Provider{
		ID:     catwalk.InferenceProvider(pc.ID),
		Name:   pc.Name,
		Type:   pc.Type,
		Models: pc.Models,
	}
	if provider.Name == "" {
		provider.Name = providerID
	}
	return m.openAuthenticationDialog(provider, selected, config.SelectedModelTypeLarge)
}

// editorCommand sets a session-scoped external editor override, or opens
// the editor when no command is given.
func (m *UI) editorCommand(args string) tea.Cmd {
	if command := strings.TrimSpace(args); command != "" {
		return func() tea.Msg {
			if err := os.Setenv("EDITOR", command); err != nil {
				return builtinResultMsg{Err: fmt.Errorf("failed to set editor: %w", err)}
			}
			return builtinResultMsg{Info: "External editor set to \"" + command + "\" for this session."}
		}
	}
	if m.isAgentBusy() {
		return util.ReportWarn("Agent is working, please wait...")
	}
	editorValue := m.textarea.Value()
	if m.bangMode {
		editorValue = "!" + editorValue
	}
	return m.openEditor(editorValue)
}

// setSkipRequests sets the permission mode without toggling.
func (m *UI) setSkipRequests(skip bool) tea.Cmd {
	if m.yoloModeCached() != skip {
		m.com.Workspace.PermissionSetSkipRequests(skip)
		m.yoloCache.set(skip)
		m.busyFetchGen++
		m.setEditorPrompt(skip)
	}
	status := "Ask When Needed"
	if skip {
		status = "Never Ask"
	}
	return util.ReportInfo("Permission mode: " + status)
}

// openModelsDialogForType opens the model picker on the requested tab.
func (m *UI) openModelsDialogForType(modelType dialog.ModelType) tea.Cmd {
	if m.dialog.ContainsDialog(dialog.ModelsID) {
		m.dialog.BringToFront(dialog.ModelsID)
		return nil
	}
	modelsDialog, err := dialog.NewModelsForType(m.com, m.state == uiOnboarding, modelType)
	if err != nil {
		return util.ReportError(err)
	}
	m.dialog.OpenDialog(modelsDialog)
	return nil
}

// openEffortPicker opens the reasoning effort selector, or toggles
// thinking mode for models that only expose a yes/no flag.
func (m *UI) openEffortPicker() tea.Cmd {
	cfg := m.com.Config()
	if cfg == nil {
		return util.ReportError(fmt.Errorf("configuration not found"))
	}
	agentCfg, ok := cfg.Agents[config.AgentCoder]
	if !ok {
		return util.ReportError(fmt.Errorf("agent configuration not found"))
	}
	model := cfg.GetModelByType(agentCfg.Model)
	if model == nil {
		return util.ReportWarn("No model selected. Run /model first.")
	}
	if len(model.ReasoningLevels) > 0 {
		return m.openReasoningDialog()
	}
	if model.CanReason {
		return m.updateAgentModelCmd(func() tea.Msg {
			cfg := m.com.Config()
			if cfg == nil {
				return util.NewErrorMsg(fmt.Errorf("configuration not found"))
			}
			agentCfg, ok := cfg.Agents[config.AgentCoder]
			if !ok {
				return util.NewErrorMsg(fmt.Errorf("agent configuration not found"))
			}
			current := cfg.Models[agentCfg.Model]
			current.Think = !current.Think
			if err := m.com.Workspace.UpdatePreferredModel(config.ScopeGlobal, agentCfg.Model, current); err != nil {
				return util.NewErrorMsg(err)
			}
			m.com.Workspace.UpdateAgentModel(context.TODO())
			status := "disabled"
			if current.Think {
				status = "enabled"
			}
			return util.NewInfoMsg("Thinking mode " + status)
		})
	}
	return util.ReportInfo("The current model has no reasoning effort settings.")
}

// tasksReport lists the in-process background shells. In client/server
// mode the shells live in the server process, so this list belongs to
// whatever process owns the TUI.
func (m *UI) tasksReport() string {
	if _, local := m.com.Workspace.(ralphWorkspace); !local {
		return "Background tasks are owned by the server process in client/server mode and are not exposed through the protocol."
	}
	mgr := shell.GetBackgroundShellManager()
	ids := mgr.List()
	if len(ids) == 0 {
		return "No background tasks."
	}
	sort.Strings(ids)

	var b strings.Builder
	fmt.Fprintf(&b, "Background tasks: %d\n", len(ids))
	for _, id := range ids {
		bg, ok := mgr.Get(id)
		if !ok {
			continue
		}
		status := "running"
		if _, _, done, _ := bg.GetOutput(); done {
			status = "completed"
		}
		label := bg.Description
		if label == "" {
			label = bg.Command
		}
		fmt.Fprintf(&b, "\n[%s] %s\n  %s\n  %s\n", id, status, label, bg.WorkingDir)
	}
	return b.String()
}

// usageReport renders token, context, budget, and quota usage.
func (m *UI) usageReport() string {
	return buildUsageReport(m.session, m.com.Config())
}

func buildUsageReport(sess *session.Session, cfg *config.Config) string {
	if sess == nil {
		return "No active session."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s", sess.Title)
	if sess.Title == "" {
		b.WriteString("(untitled)")
	}
	fmt.Fprintf(&b, "\nID: %s\n", sess.ID)
	fmt.Fprintf(&b, "Prompt tokens:     %d\n", sess.PromptTokens)
	fmt.Fprintf(&b, "Completion tokens: %d\n", sess.CompletionTokens)
	total := sess.PromptTokens + sess.CompletionTokens
	fmt.Fprintf(&b, "Total tokens:      %d\n", total)
	if sess.EstimatedUsage {
		b.WriteString("(token counts are estimated)\n")
	}
	fmt.Fprintf(&b, "Cost: $%.4f\n", sess.Cost)

	if cfg != nil {
		if model := cfg.GetModelByType(config.SelectedModelTypeLarge); model != nil && model.ContextWindow > 0 {
			pct := float64(total) / float64(model.ContextWindow) * 100
			fmt.Fprintf(&b, "Context: %d/%d tokens (%.1f%%)\n", total, model.ContextWindow, pct)
		}
		if cfg.Options != nil && cfg.Options.Budget != nil && cfg.Options.Budget.MaxUSD > 0 {
			pct := sess.Cost / cfg.Options.Budget.MaxUSD * 100
			fmt.Fprintf(&b, "Budget: $%.4f/$%.4f (%.1f%%)\n", sess.Cost, cfg.Options.Budget.MaxUSD, pct)
		}
	}
	return b.String()
}

// statusReport renders the current runtime state.
func (m *UI) statusReport() string {
	var provider, model string
	skipRequests := m.yoloModeCached()
	cfg := m.com.Config()
	if cfg != nil {
		if selected, ok := cfg.Models[config.SelectedModelTypeLarge]; ok {
			provider = selected.Provider
			model = selected.Model
		}
	}
	return buildStatusReport(
		m.session,
		provider,
		model,
		m.com.Workspace.WorkingDir(),
		skipRequests,
		m.agentReady,
		m.promptQueue,
	)
}

func buildStatusReport(
	sess *session.Session,
	provider, model, workDir string,
	skipRequests bool,
	agentReady bool,
	queued int,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Floyd %s (commit %s)\n", version.Version, version.Commit)

	sessionID, title := "(none)", "(none)"
	if sess != nil {
		sessionID = sess.ID
		if sess.Title != "" {
			title = sess.Title
		}
	}
	fmt.Fprintf(&b, "Session: %s\n", title)
	fmt.Fprintf(&b, "Session ID: %s\n", sessionID)
	fmt.Fprintf(&b, "Working directory: %s\n", workDir)
	fmt.Fprintf(&b, "Provider: %s\n", orDash(provider))
	fmt.Fprintf(&b, "Model: %s\n", orDash(model))

	mode := "Ask When Needed"
	if skipRequests {
		mode = "Never Ask"
	}
	fmt.Fprintf(&b, "Permission mode: %s\n", mode)

	ready := "not ready"
	if agentReady {
		ready = "ready"
	}
	fmt.Fprintf(&b, "Agent: %s\n", ready)
	fmt.Fprintf(&b, "Queued prompts: %d\n", queued)

	if sess != nil {
		total := sess.PromptTokens + sess.CompletionTokens
		fmt.Fprintf(&b, "Tokens: %d\n", total)
		if len(sess.Todos) > 0 {
			fmt.Fprintf(&b, "Todos: %d\n", len(sess.Todos))
		}
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// titleCommand shows the title without an argument and sets it otherwise.
func (m *UI) titleCommand(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	title := strings.TrimSpace(args)
	if title == "" {
		name := m.session.Title
		if name == "" {
			name = "(untitled)"
		}
		return m.showInfo("Session Title", name)
	}
	sess := *m.session
	sess.Title = title
	ws := m.com.Workspace
	return func() tea.Msg {
		if _, err := ws.SaveSession(context.Background(), sess); err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{Info: "Session renamed to: " + title}
	}
}

// exportMarkdown writes the current transcript as a Markdown file.
func (m *UI) exportMarkdown(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session to export.")
	}
	sess := *m.session
	ws := m.com.Workspace
	workDir := ws.WorkingDir()
	path := resolveExportPath(args, workDir, sess.ID, "md")
	return func() tea.Msg {
		msgs, err := ws.ListMessages(context.Background(), sess.ID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		md := buildSessionMarkdown(&sess, msgs, workDir)
		if err := writeExportFile(path, []byte(md)); err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{
			Title: "Export",
			Body:  fmt.Sprintf("Exported %d messages to:\n%s", len(msgs), path),
		}
	}
}

// exportDebugZip writes a diagnostic archive for the current session.
func (m *UI) exportDebugZip() tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session to export.")
	}
	sess := *m.session
	ws := m.com.Workspace
	workDir := ws.WorkingDir()
	path := resolveExportPath("", workDir, sess.ID, "zip")
	statusText := m.statusReport()
	usageText := buildUsageReport(&sess, m.com.Config())
	tasksText := m.tasksReport()
	return func() tea.Msg {
		msgs, err := ws.ListMessages(context.Background(), sess.ID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		files := map[string][]byte{
			"session.md":    []byte(buildSessionMarkdown(&sess, msgs, workDir)),
			"status.txt":    []byte(statusText),
			"usage.txt":     []byte(usageText),
			"tasks.txt":     []byte(tasksText),
			"messages.json": mustJSON(msgs),
			"metadata.json": mustJSON(map[string]any{
				"version":       version.Version,
				"commit":        version.Commit,
				"build_id":      version.BuildID,
				"session_id":    sess.ID,
				"session_title": sess.Title,
				"working_dir":   workDir,
				"exported_at":   time.Now().UTC().Format(time.RFC3339),
			}),
		}
		if err := writeExportZip(path, files); err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{
			Title: "Debug Export",
			Body:  fmt.Sprintf("Wrote diagnostic archive:\n%s", path),
		}
	}
}

func mustJSON(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

func resolveExportPath(arg, workDir, sessionID, ext string) string {
	name := fmt.Sprintf("floyd-export-%s-%s.%s",
		shortID(sessionID), time.Now().Format("20060102-150405"), ext)
	if ext == "zip" {
		name = fmt.Sprintf("floyd-debug-%s-%s.zip",
			shortID(sessionID), time.Now().Format("20060102-150405"))
	}
	if arg = strings.TrimSpace(arg); arg != "" {
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(workDir, arg)
		}
		if info, err := os.Stat(arg); err == nil && info.IsDir() {
			return filepath.Join(arg, name)
		}
		if strings.HasSuffix(arg, string(filepath.Separator)) {
			return filepath.Join(arg, name)
		}
		return arg
	}
	return filepath.Join(workDir, name)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func buildSessionMarkdown(sess *session.Session, msgs []message.Message, workDir string) string {
	var b strings.Builder
	title := sess.Title
	if title == "" {
		title = "Session"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Session: `%s`\n", sess.ID)
	fmt.Fprintf(&b, "- Working directory: `%s`\n", workDir)
	fmt.Fprintf(&b, "- Exported: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Messages: %d\n\n", len(msgs))

	for _, msg := range msgs {
		text := strings.TrimSpace(msg.Content().Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", markdownRole(msg.Role), text)
	}
	return b.String()
}

func markdownRole(role message.MessageRole) string {
	switch role {
	case message.User:
		return "User"
	case message.Assistant:
		return "Assistant"
	case message.System:
		return "System"
	default:
		return "Message"
	}
}

func writeExportFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func writeExportZip(path string, files map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := w.Write(files[name]); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

// copyLastAssistant copies the most recent assistant text to the clipboard.
func (m *UI) copyLastAssistant() tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	sessionID := m.session.ID
	ws := m.com.Workspace
	return func() tea.Msg {
		msgs, err := ws.ListMessages(context.Background(), sessionID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		text := lastAssistantText(msgs)
		if text == "" {
			return builtinResultMsg{Err: fmt.Errorf("no assistant message to copy")}
		}
		clipboard.WriteText(text)
		return builtinResultMsg{
			Clipboard: text,
			Info:      "Last assistant message copied to clipboard.",
		}
	}
}

func lastAssistantText(msgs []message.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != message.Assistant {
			continue
		}
		if text := strings.TrimSpace(msgs[i].Content().Text); text != "" {
			return text
		}
	}
	return ""
}

// ralphWorkspace is implemented by the local workspace and exposes the app
// services that have no client/server API yet.
type ralphWorkspace interface {
	App() *app.App
}

// storeWorkspace exposes the config store for /reload.
type storeWorkspace interface {
	Store() *config.ConfigStore
}

type goalAction uint8

const (
	goalShowStatus goalAction = iota
	goalCancelGoal
	goalUnsupported
	goalStart
)

// parseGoalArgs maps /goal arguments onto the supported Ralph loop
// operations. Pause, resume, replacement and queues have no chassis
// mechanism and are reported as unsupported.
func parseGoalArgs(args string) (goalAction, string) {
	args = strings.TrimSpace(args)
	switch {
	case args == "" || args == "status":
		return goalShowStatus, ""
	case args == "cancel":
		return goalCancelGoal, ""
	case args == "pause" || args == "resume":
		return goalUnsupported, ""
	case strings.HasPrefix(args, "next ") || strings.HasPrefix(args, "replace "):
		return goalUnsupported, ""
	}
	objective := strings.TrimSpace(strings.TrimPrefix(args, "--"))
	if objective == "" {
		return goalShowStatus, ""
	}
	return goalStart, objective
}

func startRalphGoal(dataDir, sessionID, objective string) error {
	_, err := ralph.New(dataDir).Start(sessionID, objective, ralph.DefaultMaxIterations, "")
	return err
}

func (m *UI) goalCommand(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session. Send a prompt first, then start a goal.")
	}
	cfg := m.com.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.DataDirectory == "" {
		return util.ReportError(fmt.Errorf("data directory is not configured"))
	}
	if _, local := m.com.Workspace.(storeWorkspace); !local {
		return util.ReportWarn("Goals require a local workspace; the server API does not expose the Ralph loop yet.")
	}
	dataDir := cfg.Options.DataDirectory
	loop := ralph.New(dataDir)
	action, objective := parseGoalArgs(args)

	switch action {
	case goalShowStatus:
		return func() tea.Msg {
			state := loop.Status()
			if state == nil {
				return builtinResultMsg{Title: "Goal", Body: "No active goal."}
			}
			body := fmt.Sprintf(
				"Iteration: %d\nMax iterations: %d\nCompletion promise: %s\nStarted: %s\n\n%s",
				state.Iteration, state.MaxIterations, orDash(state.CompletionPromise), state.StartedAt, state.Prompt,
			)
			return builtinResultMsg{Title: "Goal", Body: body}
		}
	case goalCancelGoal:
		return func() tea.Msg {
			iteration, err := loop.Cancel()
			if err != nil {
				return builtinResultMsg{Info: "No active goal to cancel."}
			}
			return builtinResultMsg{Info: fmt.Sprintf("Goal cancelled at iteration %d.", iteration)}
		}
	case goalUnsupported:
		return util.ReportInfo("Goal pause, resume, replacement and queues are not supported; use /goal cancel and /goal <objective>.")
	}

	if m.isAgentBusy() {
		return util.ReportWarn("Agent is busy, please wait before starting a goal.")
	}
	sessionID := m.session.ID
	return func() tea.Msg {
		if loop.Status() != nil {
			return builtinResultMsg{Info: "A goal is already active; use /goal cancel first."}
		}
		if err := startRalphGoal(dataDir, sessionID, objective); err != nil {
			return builtinResultMsg{Err: fmt.Errorf("failed to start goal: %w", err)}
		}
		return builtinResultMsg{
			Info: "Goal started; the Ralph loop will continue after each turn.",
			Send: objective,
		}
	}
}

func (m *UI) forkSession() tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session to fork.")
	}
	ws, ok := m.com.Workspace.(ralphWorkspace)
	if !ok {
		return util.ReportWarn("Forking requires a local workspace; the server API does not expose fork yet.")
	}
	sessionID := m.session.ID
	// The local workspace exposes the in-process app and its session service.
	appWS := ws.App()
	return func() tea.Msg {
		result, err := session.Fork(context.Background(), appWS.Sessions, sessionID, -1)
		if err != nil {
			return builtinResultMsg{Err: fmt.Errorf("failed to fork session: %w", err)}
		}
		return builtinResultMsg{
			Title: "Fork",
			Body: fmt.Sprintf(
				"Forked to session %s (%d messages).\nStill in the original session; open the fork from /sessions.",
				result.NewSession.ID, result.CopiedCount,
			),
		}
	}
}

func (m *UI) undoPrompts(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	if m.isAgentBusy() {
		return util.ReportWarn("Agent is busy, please wait before undoing prompts.")
	}
	count := 1
	if trimmed := strings.TrimSpace(args); trimmed != "" {
		n, err := strconv.Atoi(trimmed)
		if err != nil || n < 1 {
			return util.ReportWarn("Usage: /undo [count]")
		}
		count = n
	}
	ws, ok := m.com.Workspace.(ralphWorkspace)
	if !ok {
		return util.ReportWarn("Undo requires a local workspace; the server API does not expose message deletion yet.")
	}
	appWS := ws.App()
	sessionID := m.session.ID
	return func() tea.Msg {
		ctx := context.Background()
		msgs, err := appWS.Messages.List(ctx, sessionID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		var userIndices []int
		for i, msg := range msgs {
			if msg.Role == message.User {
				userIndices = append(userIndices, i)
			}
		}
		if len(userIndices) < count {
			return builtinResultMsg{Err: fmt.Errorf("only %d prompt(s) available to undo", len(userIndices))}
		}
		cutoff := userIndices[len(userIndices)-count]
		deleted := 0
		for _, msg := range msgs[cutoff:] {
			if err := appWS.Messages.Delete(ctx, msg.ID); err != nil {
				return builtinResultMsg{Err: err}
			}
			deleted++
		}
		return builtinResultMsg{
			Info: fmt.Sprintf("Withdrew %d prompt(s) (%d messages).", count, deleted),
		}
	}
}

func (m *UI) logoutProvider(args string) tea.Cmd {
	cfg := m.com.Config()
	if cfg == nil {
		return util.ReportError(fmt.Errorf("configuration not found"))
	}

	provider := strings.ToLower(strings.TrimSpace(args))
	switch provider {
	case "github", "github-copilot":
		provider = "copilot"
	}
	if provider == "" {
		var loggedIn []string
		for id, p := range cfg.Providers.Copy() {
			if p.OAuthToken != nil || p.APIKey != "" {
				switch id {
				case "hyper", "copilot":
					loggedIn = append(loggedIn, id)
				}
			}
		}
		sort.Strings(loggedIn)
		switch len(loggedIn) {
		case 0:
			return m.showInfo("Logout", "Not logged in to any OAuth provider.")
		case 1:
			provider = loggedIn[0]
		default:
			return m.showInfo("Logout", "Logged in to: "+strings.Join(loggedIn, ", ")+"\nUse /logout <provider>.")
		}
	}
	if provider != "hyper" && provider != "copilot" {
		return util.ReportWarn("Unknown provider: " + provider + " (available: hyper, copilot)")
	}

	ws := m.com.Workspace
	return func() tea.Msg {
		ctx := context.Background()
		removed := 0
		var lastErr error
		for _, key := range []string{
			"providers." + provider + ".api_key",
			"providers." + provider + ".oauth",
		} {
			if err := ws.RemoveConfigField(config.ScopeGlobal, key); err != nil {
				lastErr = err
				continue
			}
			removed++
		}
		if removed == 0 {
			return builtinResultMsg{Err: fmt.Errorf("failed to log out of %s: %w", provider, lastErr)}
		}
		if err := ws.UpdateAgentModel(ctx); err != nil {
			return builtinResultMsg{Info: "Logged out of " + provider + ", but model refresh failed: " + err.Error()}
		}
		return builtinResultMsg{Info: "Logged out of " + provider + "."}
	}
}

func (m *UI) reloadConfig() tea.Cmd {
	storeWS, ok := m.com.Workspace.(storeWorkspace)
	if !ok {
		return util.ReportWarn("Reload requires a local workspace; the server API does not expose config reload yet.")
	}
	baseWS := m.com.Workspace
	store := storeWS.Store()
	return func() tea.Msg {
		ctx := context.Background()
		if err := store.ReloadFromDisk(ctx); err != nil {
			return builtinResultMsg{Err: fmt.Errorf("failed to reload configuration: %w", err)}
		}
		if err := baseWS.UpdateAgentModel(ctx); err != nil {
			return builtinResultMsg{Info: "Configuration reloaded, but model refresh failed: " + err.Error()}
		}
		return builtinResultMsg{Info: "Configuration reloaded.", ReloadTUI: true}
	}
}

// reapplyTUI pushes the current configuration's UI preferences into the
// running model. Called after /reload and /reload-tui.
func (m *UI) reapplyTUI() {
	cfg := m.com.Config()
	if cfg == nil {
		return
	}
	providerID := cfg.Models[config.SelectedModelTypeLarge].Provider
	m.themeKey = ""
	m.applyThemeForProvider(providerID)
	m.notifyBackend = selectNotificationBackend(m.caps, cfg)
	m.isTransparent = cfg.Options != nil && cfg.Options.TUI.IsTransparent()
	m.updateLayoutAndSize()
}

// runBtw runs a one-shot side completion in a temporary session that is
// deleted afterwards, so the current session's transcript is untouched.
func (m *UI) runBtw(question string) tea.Cmd {
	question = strings.TrimSpace(question)
	if question == "" {
		return m.promptBuiltinInput("btw")
	}
	if err := m.com.Workspace.AgentReadyErr(); err != nil {
		return util.ReportError(err)
	}
	ws := m.com.Workspace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		temp, err := ws.CreateSession(ctx, "btw")
		if err != nil {
			return builtinResultMsg{Err: fmt.Errorf("failed to create side session: %w", err)}
		}
		defer ws.DeleteSession(context.Background(), temp.ID) //nolint:errcheck

		if err := ws.AgentRun(ctx, temp.ID, question); err != nil {
			return builtinResultMsg{Err: fmt.Errorf("side completion failed: %w", err)}
		}
		waitForSessionAnswer(ctx, ws, temp.ID)
		msgs, err := ws.ListMessages(ctx, temp.ID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		answer := lastAssistantText(msgs)
		if answer == "" {
			return builtinResultMsg{Err: fmt.Errorf("side session produced no answer")}
		}
		return builtinResultMsg{Title: "btw", Body: answer}
	}
}

// waitForSessionAnswer waits until the session is running (or already
// produced an answer) and then until it stops running. Client mode
// accepts the prompt asynchronously, so the first check avoids reading
// the transcript before the server starts the turn.
func waitForSessionAnswer(ctx context.Context, ws workspace.Workspace, sessionID string) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ws.AgentIsSessionBusy(sessionID) {
			break
		}
		if msgs, err := ws.ListMessages(ctx, sessionID); err == nil && lastAssistantText(msgs) != "" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	for ws.AgentIsSessionBusy(sessionID) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// waitForSessionIdle polls a session until it stops running. Local mode
// returns from AgentRun only after the turn, so this is mostly a no-op
// there; client mode accepts the prompt asynchronously.
func waitForSessionIdle(ctx context.Context, ws workspace.Workspace, sessionID string) {
	time.Sleep(200 * time.Millisecond)
	for ws.AgentIsSessionBusy(sessionID) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// runSwarm runs one task through every configured agent name in a
// separate session and collects the outputs. The chassis dispatches all
// names through the same coordinator (as the CLI swarm command notes).
func (m *UI) runSwarm(task string) tea.Cmd {
	task = strings.TrimSpace(task)
	if task == "" {
		return m.promptBuiltinInput("swarm")
	}
	cfg := m.com.Config()
	if cfg == nil || len(cfg.Agents) == 0 {
		return util.ReportWarn("No agents are configured.")
	}
	names := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		names = append(names, name)
	}
	sort.Strings(names)

	ws := m.com.Workspace
	return func() tea.Msg {
		type outcome struct {
			name      string
			sessionID string
			output    string
			err       error
		}
		results := make([]outcome, len(names))
		var wg sync.WaitGroup
		for i, name := range names {
			wg.Add(1)
			go func(i int, name string) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				result := outcome{name: name}
				sess, err := ws.CreateSession(ctx, "swarm: "+name)
				if err != nil {
					result.err = err
					results[i] = result
					return
				}
				result.sessionID = sess.ID
				if err := ws.AgentRun(ctx, sess.ID, task); err != nil {
					result.err = err
					results[i] = result
					return
				}
				waitForSessionIdle(ctx, ws, sess.ID)
				msgs, err := ws.ListMessages(ctx, sess.ID)
				if err != nil {
					result.err = err
					results[i] = result
					return
				}
				result.output = lastAssistantText(msgs)
				results[i] = result
			}(i, name)
		}
		wg.Wait()

		var b strings.Builder
		for _, result := range results {
			fmt.Fprintf(&b, "== %s (session %s) ==\n", result.name, result.sessionID)
			if result.err != nil {
				fmt.Fprintf(&b, "error: %v\n\n", result.err)
				continue
			}
			if result.output == "" {
				result.output = "(no output)"
			}
			b.WriteString(result.output)
			b.WriteString("\n\n")
		}
		return builtinResultMsg{Title: "Swarm", Body: b.String()}
	}
}
