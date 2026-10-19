package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/commands"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

const (
	wantSkills  = 41
	wantAliases = 2
)

func TestInstallCommands(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, InstallCommands(dataDir))

	commands, err := commandFiles()
	require.NoError(t, err)
	require.Len(t, commands, wantSkills)

	aliases, err := aliasFiles(commands)
	require.NoError(t, err)
	require.Len(t, aliases, wantAliases)
	require.Equal(t, "branch.md", aliases["fork.md"], "fork must copy branch.md")
	require.Equal(t, "arena.md", aliases["race.md"], "race must copy arena.md")

	commandsDir := filepath.Join(dataDir, "commands")
	entries, err := os.ReadDir(commandsDir)
	require.NoError(t, err)
	require.Len(t, entries, wantSkills+wantAliases)

	for name, want := range commands {
		got, readErr := os.ReadFile(filepath.Join(commandsDir, name))
		require.NoError(t, readErr, name)
		require.NotEmpty(t, got, name)
		require.Equal(t, want, got, name)
	}
	for name, target := range aliases {
		got, readErr := os.ReadFile(filepath.Join(commandsDir, name))
		require.NoError(t, readErr, name)
		require.NotEmpty(t, got, name)
		require.Equal(t, commands[target], got, name)
	}
}

func TestInstallCommandsIdempotent(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, InstallCommands(dataDir))

	path := filepath.Join(dataDir, "commands", "branch.md")
	expected, err := os.ReadFile(path)
	require.NoError(t, err)

	old := time.Unix(1000, 0)
	require.NoError(t, os.Chtimes(path, old, old))

	require.NoError(t, InstallCommands(dataDir))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, old.Unix(), info.ModTime().Unix(), "unchanged content must not be rewritten")

	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o644))
	require.NoError(t, InstallCommands(dataDir))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, got, "changed content must be rewritten")
}

func TestInstallCommandsReadOnlyDataDir(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.Mkdir(dataDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o755) })

	err := InstallCommands(dataDir)
	require.Error(t, err)
}

// TestInstalledCommandsLoad proves the runtime loader sees every installed
// skill and alias as a project command.
func TestInstalledCommandsLoad(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, InstallCommands(dataDir))

	cfg := &config.Config{Options: &config.Options{DataDirectory: dataDir}}
	found, err := commands.LoadCustomCommands(cfg)
	require.NoError(t, err)

	projectCommands := make(map[string]bool)
	for _, command := range found {
		if strings.HasPrefix(command.ID, "project:") {
			projectCommands[command.ID] = true
		}
	}

	require.Len(t, projectCommands, wantSkills+wantAliases)
	require.True(t, projectCommands["project:branch"])
	require.True(t, projectCommands["project:fork"])
	require.True(t, projectCommands["project:arena"])
	require.True(t, projectCommands["project:race"])
}
