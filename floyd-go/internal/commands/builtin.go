package commands

import "slices"

// BuiltinCommand describes a core slash command shipped with Floyd. The
// UI uses these entries to build the command palette and the slash
// dispatcher; the actual handlers live in internal/ui/model.
type BuiltinCommand struct {
	// Name is the command name without the leading slash.
	Name string
	// Aliases are alternative names that resolve to the same command.
	Aliases []string
	// Description is the human-readable job of the command.
	Description string
	// ArgumentHint documents the arguments the command accepts.
	ArgumentHint string
}

// builtinCommands is the shipped group-1 command surface. Keep this list
// and the dispatcher in internal/ui/model/builtin.go in sync; the tests in
// both packages enforce it.
var builtinCommands = []BuiltinCommand{
	{Name: "yolo", Aliases: []string{"yes"}, Description: "Toggle YOLO mode (skip permission prompts)"},
	{Name: "auto", Description: "Set Never Ask permission mode (skip all prompts)"},
	{Name: "permission", Description: "Select the permission mode"},
	{Name: "settings", Aliases: []string{"config"}, Description: "Show configuration sources and audit"},
	{Name: "model", Description: "Switch the large model"},
	{Name: "secondary-model", Aliases: []string{"subagent-model"}, Description: "Select the small (subagent) model"},
	{Name: "effort", Aliases: []string{"thinking"}, Description: "Select the reasoning effort"},
	{Name: "provider", Aliases: []string{"providers"}, Description: "Browse configured providers and models"},
	{Name: "help", Aliases: []string{"h", "?"}, Description: "Toggle the keyboard help"},
	{Name: "new", Aliases: []string{"clear"}, Description: "Start a new session"},
	{Name: "sessions", Aliases: []string{"resume"}, Description: "Browse and resume sessions"},
	{Name: "tasks", Aliases: []string{"task"}, Description: "List background tasks"},
	{Name: "mcp", Description: "Show MCP servers and their status"},
	{Name: "plugins", Description: "Open the plugins library"},
	{Name: "compact", Description: "Compact the conversation context", ArgumentHint: "[instruction]"},
	{Name: "goal", Description: "Start or manage an autonomous Ralph goal", ArgumentHint: "status|cancel|<objective>"},
	{Name: "init", Description: "Analyze the project and generate AGENTS.md"},
	{Name: "fork", Description: "Fork the current session"},
	{Name: "title", Aliases: []string{"rename"}, Description: "Show or set the session title", ArgumentHint: "[title]"},
	{Name: "usage", Description: "Show token, context, and budget usage"},
	{Name: "status", Description: "Show session and runtime status"},
	{Name: "undo", Description: "Withdraw the last prompt", ArgumentHint: "[count]"},
	{Name: "editor", Description: "Open the external editor"},
	{Name: "login", Description: "Authenticate a provider through the model picker"},
	{Name: "logout", Description: "Log out of a configured provider", ArgumentHint: "[provider]"},
	{Name: "export-md", Aliases: []string{"export"}, Description: "Export the session as Markdown", ArgumentHint: "[path]"},
	{Name: "export-debug-zip", Description: "Export a diagnostic ZIP for this session"},
	{Name: "copy", Description: "Copy the last assistant message"},
	{Name: "exit", Aliases: []string{"quit", "q"}, Description: "Exit Floyd"},
	{Name: "version", Description: "Show version information"},
	{Name: "swarm", Description: "Dispatch one task across configured agents (local mode)", ArgumentHint: "<task>"},
	{Name: "btw", Description: "Ask a one-shot side question without touching this session", ArgumentHint: "<question>"},
	{Name: "feedback", Aliases: []string{"bug"}, Description: "Open the issue tracker to report a problem"},
	{Name: "reload", Description: "Reload configuration from disk (local mode)"},
	{Name: "reload-tui", Description: "Reapply UI styles from the current configuration"},
	{Name: "web", Description: "Start or reuse the web UI and show its URL"},
	{Name: "remote-control", Aliases: []string{"rc"}, Description: "Start or reuse the web UI through the Floyd relay"},
}

// BlockedBuiltin documents a required command that cannot be shipped as a
// real trigger because the chassis has no mechanism for it. The value is
// the evidence-based reason.
var BlockedBuiltin = map[string]string{
	"plan":        "no plan mode, plan storage, or plan file support exists; the chassis only has the agent-driven todos tool, which neither gates tools nor persists a plan",
	"add-dir":     "no additional workspace directory concept exists in config or the workspace interface; every file tool resolves against the single WorkingDir",
	"experiments": "no experimental feature flag system exists; the only 'experimental' reference is an MCP capability key",
	"theme":       "only one effective theme exists (HyperfloydObsidiana returns CharmtonePantera); there is no theme picker or persisted theme setting",
	"desktop":     "no desktop or install URL is documented for this private fork; the reference URL belongs to the upstream product",
}

// ExcludedBuiltin would list commands owned by another lane. The remote
// lane's commands are now registered, so no command is excluded.
var ExcludedBuiltin = map[string]string{}

// Builtins returns a copy of the registered builtin command list.
func Builtins() []BuiltinCommand {
	return slices.Clone(builtinCommands)
}

// BuiltinNames returns the registered command names in registry order.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtinCommands))
	for _, cmd := range builtinCommands {
		names = append(names, cmd.Name)
	}
	return names
}

// Builtin resolves a command name or alias. Names are matched exactly.
func Builtin(name string) (BuiltinCommand, bool) {
	for _, cmd := range builtinCommands {
		if cmd.Name == name {
			return cmd, true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd, true
			}
		}
	}
	return BuiltinCommand{}, false
}

// IsBlocked reports whether name is a documented blocked command.
func IsBlocked(name string) bool {
	_, ok := BlockedBuiltin[name]
	return ok
}

// IsExcluded reports whether name is owned by another lane.
func IsExcluded(name string) bool {
	_, ok := ExcludedBuiltin[name]
	return ok
}
