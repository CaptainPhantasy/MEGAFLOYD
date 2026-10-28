package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

// TestCanonicalScratchpadAndJobsIsolation pins the decided behaviour that
// per-project agent state is account-private: the scratchpad and job records
// live in the caller's own workspace store, never in the shared project
// directory, even when no explicit data directory is configured.
func TestCanonicalScratchpadAndJobsIsolation(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "1")
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	dir := t.TempDir()
	private, err := megafloyd.WorkspaceDataDir(dir)
	require.NoError(t, err)
	require.False(t, strings.HasPrefix(private, dir), "workspace store must live outside the project directory")

	legacy := filepath.Join(dir, ".floyd", "scratchpad.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o700))
	require.NoError(t, os.WriteFile(legacy, []byte("PARENT_STATE"), 0o600))
	shared := filepath.Join(dir, ".megafloyd")
	require.NoError(t, os.MkdirAll(shared, 0o700))

	tool := NewManageScratchpadTool(dir)
	read, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "read", Name: "manage_scratchpad", Input: "{\"action\":\"read\"}"})
	require.NoError(t, err)
	require.NotContains(t, read.Content, "PARENT_STATE")
	written, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "write", Name: "manage_scratchpad", Input: "{\"action\":\"write\",\"content\":\"CANONICAL_STATE\"}"})
	require.NoError(t, err)
	require.False(t, written.IsError)
	content, err := os.ReadFile(filepath.Join(private, "scratchpad.md"))
	require.NoError(t, err)
	require.Equal(t, "CANONICAL_STATE", string(content))
	content, err = os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, "PARENT_STATE", string(content))

	root, err := jobsDataDirectory("", dir)
	require.NoError(t, err)
	require.Equal(t, private, root, "job records follow the account-private workspace store")

	contract := NewContractTool("", nil, dir)
	created := runJobTool(t, contract, t.Context(), ContractParams{
		Action:       "create",
		Objective:    "Keep job records private",
		Requirements: []string{"Records are account-private"},
	})
	meta := decodeContract(t, created)
	require.NotEmpty(t, meta.JobID)
	require.Equal(t, filepath.Join(private, "jobs", meta.JobID), meta.Path)
	require.DirExists(t, meta.Path)

	entries, err := os.ReadDir(shared)
	require.NoError(t, err)
	require.Empty(t, entries, "no scratchpad or job state may land in the shared project directory")
}
