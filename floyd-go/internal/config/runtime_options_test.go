package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeOptionsAreAtomicAndDoNotWriteConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := &Config{Permissions: &Permissions{}}
	cfg.setDefaults(root, filepath.Join(root, "data"))
	s := &ConfigStore{config: cfg, workingDir: root}
	mode := "manual"
	err := s.ApplyRuntimeOptions(RuntimeOptions{PermissionMode: &mode, AllowedTools: []string{"bash"}, AdditionalDirs: []string{"missing"}})
	require.Error(t, err)
	require.NotContains(t, s.Config().Permissions.AllowedTools, "bash")
	extra := filepath.Join(root, "extra")
	require.NoError(t, os.Mkdir(extra, 0o700))
	require.NoError(t, s.ApplyRuntimeOptions(RuntimeOptions{PermissionMode: &mode, AllowedTools: []string{"bash"}, AdditionalDirs: []string{"extra"}}))
	require.Equal(t, []string{"bash"}, s.Config().Permissions.AllowedTools)
	canonical, err := filepath.EvalSymlinks(extra)
	require.NoError(t, err)
	require.Contains(t, s.Config().Options.AdditionalDirs, canonical)
	require.Equal(t, mode, *s.Overrides().PermissionMode)
	_, err = os.Stat(filepath.Join(root, "data", "floyd.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
