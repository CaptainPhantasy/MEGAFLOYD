package commands

import "slices"

// BuiltinCommand describes a core slash command shipped with MegaFloyd. The
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
	{Name: "yolo", Aliases: []string{"yes"}, Description: "Toggle automatic permission for ordinary tools"},
	{Name: "auto", Description: "Allow ordinary tools; protected confirmations still apply"},
	{Name: "permission", Description: "Select the permission mode"},
	{Name: "settings", Aliases: []string{"config"}, Description: "Show configuration sources and audit"},
	{Name: "model", Description: "Switch the large model", ArgumentHint: "[provider/model|name]"},
	{Name: "secondary-model", Aliases: []string{"subagent-model"}, Description: "Select the small (subagent) model", ArgumentHint: "[provider/model|name]"},
	{Name: "effort", Aliases: []string{"thinking"}, Description: "Select the reasoning effort"},
	{Name: "provider", Aliases: []string{"providers"}, Description: "Browse configured providers and models"},
	{Name: "help", Aliases: []string{"h", "?"}, Description: "Toggle the keyboard help"},
	{Name: "new", Aliases: []string{"clear"}, Description: "Start a new session"},
	{Name: "plan", Description: "Enter or manage plan mode", ArgumentHint: "status|clear|cancel"},
	{Name: "sessions", Aliases: []string{"resume"}, Description: "Browse and resume sessions"},
	{Name: "tasks", Aliases: []string{"task"}, Description: "Inspect or control background tasks", ArgumentHint: "[id|cancel id|detach id]"},
	{Name: "mcp", Description: "Show MCP servers and their status"},
	{Name: "plugins", Description: "Open the plugins library"},
	{Name: "add-dir", Description: "Add or list an extra workspace directory", ArgumentHint: "[list]|<path>"},
	{Name: "experiments", Aliases: []string{"experimental"}, Description: "List or toggle experimental features", ArgumentHint: "[<id> [on|off]]"},
	{Name: "theme", Description: "Select the terminal UI theme", ArgumentHint: "auto|dark|light"},
	{Name: "compact", Description: "Compact the conversation context", ArgumentHint: "[instruction]"},
	{Name: "goal", Description: "Start or manage an autonomous Ralph goal", ArgumentHint: "status|cancel|<objective>"},
	{Name: "init", Description: "Initialize project guidance and records in this conversation"},
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
	{Name: "exit", Aliases: []string{"quit", "q"}, Description: "Exit MegaFloyd"},
	{Name: "version", Description: "Show version information"},
	{Name: "swarm", Description: "Show or set parallel subagent dispatch for this session", ArgumentHint: "[on|off|status|parallel N|run <task>]"},
	{Name: "btw", Description: "Ask a one-shot side question without touching this session", ArgumentHint: "<question>"},
	{Name: "feedback", Aliases: []string{"bug"}, Description: "Open the issue tracker to report a problem"},
	{Name: "reload", Description: "Reload workspace configuration from disk"},
	{Name: "reload-tui", Description: "Reapply UI styles from the current configuration"},
	{Name: "web", Description: "Open this workspace and session in the browser"},
	{Name: "desktop", Description: "Open this workspace and session in MegaFloyd desktop"},
	{Name: "tui", Description: "Keep working in this terminal session"},
	{Name: "remote-control", Aliases: []string{"rc"}, Description: "Start or reuse the web UI through the MegaFloyd relay"},
	{Name: "hooks", Description: "Inspect hook rules and recorded execution", ArgumentHint: "[explain <event>|trace]"},
	{Name: "rewind", Description: "Restore files and conversation to an earlier turn", ArgumentHint: "[turn-count]"},
	{Name: "file-history", Description: "Browse saved file versions", ArgumentHint: "[version-id]"},
	{Name: "release-notes", Aliases: []string{"changelog"}, Description: "Read versioned changes for this build"},
	{Name: "todos", Description: "Show the current session todo list"},
	{Name: "agents", Description: "Open the agent library"},
	{Name: "skills", Description: "Open the skills library"},
	{Name: "doctor", Description: "Check installation, configuration and environment health"},
	{Name: "prompt", Aliases: []string{"templates"}, Description: "Insert an editable prompt template", ArgumentHint: "[name] [JSON values]"},
	{Name: "codebase", Description: "Analyze dependencies and search workspace code", ArgumentHint: "[analyze|deps|search <text>]"},
	{Name: "session", Description: "Export or import a session with the engine", ArgumentHint: "export|import <id>"},
	{Name: "import", Description: "Import a saved engine session", ArgumentHint: "<id>"},
	{Name: "telemetry", Description: "Inspect local usage telemetry", ArgumentHint: "[report|collect|ship]"},
	{Name: "memory", Description: "Inspect the workspace memory checkpoint"},
	{Name: "context", Description: "Inspect recorded context and selected model limits"},
	{Name: "cost", Description: "Inspect session token usage and cost"},
	{Name: "vim", Description: "Toggle Vim editing in the prompt", ArgumentHint: "[on|off|status]"},
	{Name: "terminal-setup", Description: "Set terminal send and newline keys", ArgumentHint: "[standard|multiline]"},
	{Name: "output-style", Description: "Set assistant text presentation", ArgumentHint: "[markdown|plain]"},
	{Name: "privacy-settings", Description: "Inspect or change analytics preferences", ArgumentHint: "[status|on|off]"},
}

// BlockedBuiltin documents a required command that cannot be shipped as a
// real trigger because the chassis has no mechanism for it. The value is
// the evidence-based reason.
var BlockedBuiltin = map[string]string{}

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
