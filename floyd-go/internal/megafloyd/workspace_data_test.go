package megafloyd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalWorkspaceAccountsDoNotShareProjectState(t *testing.T) {
	clearEnv(t)
	project := t.TempDir()
	legacy := filepath.Join(project, ".megafloyd")
	require.NoError(t, os.MkdirAll(legacy, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "session-marker"), []byte("legacy session"), 0o600))
	firstHome, secondHome := t.TempDir(), t.TempDir()
	t.Setenv(HomeEnv, firstHome)
	first, err := WorkspaceDataDir(project)
	require.NoError(t, err)
	require.NotEqual(t, legacy, first)
	require.NoError(t, os.MkdirAll(first, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(first, "session-marker"), []byte("first account"), 0o600))
	t.Setenv(HomeEnv, secondHome)
	second, err := WorkspaceDataDir(project)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	info, err := os.Stat(second)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	_, err = os.Stat(filepath.Join(second, "session-marker"))
	require.ErrorIs(t, err, os.ErrNotExist)
	alias := filepath.Join(t.TempDir(), "project link")
	require.NoError(t, os.Symlink(project, alias))
	throughAlias, err := WorkspaceDataDir(alias)
	require.NoError(t, err)
	require.Equal(t, second, throughAlias)

	t.Setenv(HomeEnv, firstHome)
	cwd, err := filepath.EvalSymlinks(project)
	require.NoError(t, err)
	registry := filepath.Join(firstHome, "state", "workspace-data.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(registry), 0o700))
	data, err := json.Marshal(map[string]any{"version": 1, "workspaces": map[string]string{cwd: legacy}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(registry, data, 0o600))
	preserved, err := WorkspaceDataDir(project)
	require.NoError(t, err)
	require.Equal(t, legacy, preserved)
	t.Setenv(HomeEnv, secondHome)
	separate, err := WorkspaceDataDir(project)
	require.NoError(t, err)
	require.Equal(t, second, separate)
	t.Setenv(HomeEnv, firstHome)
	require.NoError(t, os.WriteFile(registry, []byte("invalid registry"), 0o600))
	_, err = WorkspaceDataDir(project)
	require.ErrorContains(t, err, "private workspace registry")
}
