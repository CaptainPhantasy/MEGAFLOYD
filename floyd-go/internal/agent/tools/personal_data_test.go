package tools

import (
	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/legacy-ai/floyd/internal/workflow"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPersonalToolsUsePrivateData(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "1")
	cwd := t.TempDir()
	shared := filepath.Join(cwd, ".megafloyd")
	require.NoError(t, os.MkdirAll(shared, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(shared, "scratchpad.md"), []byte("OTHER_ACCOUNT"), 0600))
	var first string
	for _, label := range []string{"ALICE_PRIVATE", "BOB_PRIVATE"} {
		t.Setenv("MEGAFLOYD_HOME", t.TempDir())
		tool := NewManageScratchpadTool(cwd)
		result, err := tool.Run(t.Context(), fantasy.ToolCall{Input: `{"action":"read"}`})
		require.NoError(t, err)
		require.NotContains(t, result.Content, "OTHER_ACCOUNT")
		require.NotContains(t, result.Content, "ALICE_PRIVATE")
		result, err = tool.Run(t.Context(), fantasy.ToolCall{Input: `{"action":"write","content":"` + label + `"}`})
		require.NoError(t, err)
		require.False(t, result.IsError)
		dir, err := megafloyd.WorkspaceDataDir(cwd)
		require.NoError(t, err)
		if first != "" {
			require.NotEqual(t, first, dir)
		}
		first = dir
		b, err := os.ReadFile(filepath.Join(dir, "scratchpad.md"))
		require.NoError(t, err)
		require.Equal(t, label, string(b))
		state := workflow.NewCheckpointService(cwd)
		prior, err := state.List("")
		require.NoError(t, err)
		require.Empty(t, prior)
		cp := &workflow.Checkpoint{ID: label, WorkflowName: "private"}
		require.NoError(t, state.Save(cp))
		got, err := state.Load(label)
		require.NoError(t, err)
		require.Equal(t, label, got.ID)
		require.FileExists(t, filepath.Join(dir, "workflows", label+".json"))
	}
	b, err := os.ReadFile(filepath.Join(shared, "scratchpad.md"))
	require.NoError(t, err)
	require.Equal(t, "OTHER_ACCOUNT", string(b))
}
