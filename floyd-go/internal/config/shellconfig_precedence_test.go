package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

// TestShellConfigDotFloydrcTakesPrecedence verifies that a project-local
// .floydrc overrides floydrc in the same directory on conflicting settings.
func TestShellConfigDotFloydrcTakesPrecedence(t *testing.T) {
	isolated := t.TempDir()
	t.Setenv("HOME", isolated)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(isolated, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(isolated, ".local", "share"))

	workDir := t.TempDir()
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "floydrc"),
		[]byte("option notifications bell\n"), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, ".floydrc"),
		[]byte("option notifications osc\n"), 0o644,
	))

	store, err := config.Load(workDir, dataDir, false)
	require.NoError(t, err)
	require.Equal(t, "osc", store.Config().Options.Notifications,
		".floydrc should win over floydrc")
}
