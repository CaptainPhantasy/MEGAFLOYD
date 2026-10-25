package prompt

import (
	"context"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/megafloyd/memory"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPromptCacheIsolation(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "1")
	cwd := t.TempDir()
	for _, name := range []string{".floyd", ".megafloyd"} {
		require.NoError(t, os.MkdirAll(filepath.Join(cwd, name), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(cwd, name, ".supercache"), []byte("OTHER_ACCOUNT_MEMORY"), 0600))
	}
	var first string
	for _, id := range []string{"account-a", "account-b"} {
		t.Setenv("MEGAFLOYD_HOME", t.TempDir())
		require.Empty(t, readSupercache(cwd))
		dataDir, err := megafloyd.WorkspaceDataDir(cwd)
		require.NoError(t, err)
		if first != "" {
			require.NotEqual(t, first, dataDir)
		}
		first = dataDir
		require.NoError(t, (memory.Writer{CWD: cwd, DataDir: dataDir}).Write(context.Background(), memory.Checkpoint{SessionID: id, Git: &memory.GitState{}}))
		require.Contains(t, readSupercache(cwd), id)
		cfg := config.Config{Options: &config.Options{DataDirectory: dataDir}}
		require.Contains(t, DynamicContext(context.Background(), cwd, cfg), id)
		require.NotContains(t, DynamicContext(context.Background(), cwd, cfg), "OTHER_ACCOUNT_MEMORY")
	}
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	home := os.Getenv("MEGAFLOYD_HOME")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "state"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "state", "workspace-data.json"), []byte("broken"), 0600))
	require.Contains(t, readSupercache(cwd), "personal memory unavailable")
	require.Error(t, (memory.Writer{CWD: cwd}).Write(context.Background(), memory.Checkpoint{}))
}
