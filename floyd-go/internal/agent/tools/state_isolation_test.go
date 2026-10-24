package tools

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

func TestCanonicalScratchpadAndJobsIsolation(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "1")
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".floyd", "scratchpad.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o700))
	require.NoError(t, os.WriteFile(legacy, []byte("PARENT_STATE"), 0o600))
	require.Equal(t, filepath.Join(dir, ".megafloyd"), jobsDataDirectory("", dir))
	tool := NewManageScratchpadTool(dir)
	read, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "read", Name: "manage_scratchpad", Input: "{\"action\":\"read\"}"})
	require.NoError(t, err)
	require.NotContains(t, read.Content, "PARENT_STATE")
	written, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "write", Name: "manage_scratchpad", Input: "{\"action\":\"write\",\"content\":\"CANONICAL_STATE\"}"})
	require.NoError(t, err)
	require.False(t, written.IsError)
	content, err := os.ReadFile(filepath.Join(dir, ".megafloyd", "scratchpad.md"))
	require.NoError(t, err)
	require.Equal(t, "CANONICAL_STATE", string(content))
	content, err = os.ReadFile(legacy)
	require.NoError(t, err)
	require.Equal(t, "PARENT_STATE", string(content))
}
