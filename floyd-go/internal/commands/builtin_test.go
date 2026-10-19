package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// requiredBuiltins is the group-1 slash-command list from the audit.
var requiredBuiltins = []string{
	"yolo", "auto", "permission", "settings", "plan", "model",
	"secondary-model", "effort", "provider", "help", "new", "sessions",
	"tasks", "mcp", "plugins", "add-dir", "experiments", "reload",
	"reload-tui", "compact", "goal", "init", "fork", "title", "usage",
	"status", "undo", "editor", "theme", "login", "logout", "export-md",
	"export-debug-zip", "copy", "exit", "version", "swarm", "btw",
	"desktop", "feedback", "web", "remote-control",
}

func TestBuiltinRegistryCoversRequiredList(t *testing.T) {
	t.Parallel()

	require.Len(t, requiredBuiltins, 42, "the audited list now includes /web and /remote-control")
	require.Len(t, builtinCommands, 38)
	require.Len(t, BlockedBuiltin, 4)
	require.Empty(t, ExcludedBuiltin)

	registered := make(map[string]bool, len(builtinCommands))
	for _, cmd := range builtinCommands {
		require.NotEmpty(t, cmd.Name)
		require.NotContains(t, cmd.Name, "/")
		require.NotEmpty(t, cmd.Description)
		require.False(t, registered[cmd.Name], "duplicate command %q", cmd.Name)
		registered[cmd.Name] = true
	}

	for _, name := range requiredBuiltins {
		_, isRegistered := registered[name]
		_, isBlocked := BlockedBuiltin[name]
		_, isExcluded := ExcludedBuiltin[name]
		require.True(t, isRegistered || isBlocked || isExcluded,
			"required command %q is neither registered, blocked, nor excluded", name)
	}

	for name := range registered {
		require.Contains(t, requiredBuiltins, name,
			"registered command %q is not in the audited list", name)
	}
}

func TestBuiltinBlockedAndExcludedAreDisjoint(t *testing.T) {
	t.Parallel()

	for name, reason := range BlockedBuiltin {
		require.NotEmpty(t, reason, "blocked command %q needs evidence", name)
		_, registered := Builtin(name)
		require.False(t, registered, "blocked command %q must not be registered", name)
		require.False(t, IsExcluded(name), "blocked command %q must not also be excluded", name)
	}

	for name, reason := range ExcludedBuiltin {
		require.NotEmpty(t, reason)
		_, registered := Builtin(name)
		require.False(t, registered, "excluded command %q must not be registered", name)
	}
}

func TestBuiltinAliasesResolve(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string)
	for _, cmd := range builtinCommands {
		seen[cmd.Name] = cmd.Name
		for _, alias := range cmd.Aliases {
			require.NotContains(t, seen, alias, "alias %q is already used", alias)
			seen[alias] = cmd.Name
		}
	}

	for alias, name := range map[string]string{
		"yes":            "yolo",
		"config":         "settings",
		"subagent-model": "secondary-model",
		"clear":          "new",
		"resume":         "sessions",
		"rename":         "title",
		"export":         "export-md",
		"bug":            "feedback",
		"q":              "exit",
		"rc":             "remote-control",
	} {
		cmd, ok := Builtin(alias)
		require.True(t, ok, "alias %q should resolve", alias)
		require.Equal(t, name, cmd.Name, "alias %q", alias)
	}
}
