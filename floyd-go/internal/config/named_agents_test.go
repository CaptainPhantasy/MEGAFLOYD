package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamedAgentsSurviveConfigurationLoadAndSetup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_CONFIG", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_DATA", t.TempDir())
	dir := t.TempDir()
	raw := `{"options":{"disable_default_providers":true,"disable_provider_auto_update":true},"providers":{"fixture":{"type":"openai-compat","base_url":"http://127.0.0.1:9/v1","api_key":"fixture","models":[{"id":"model","name":"Fixture","context_window":32768,"default_max_tokens":128}]}},"models":{"large":{"provider":"fixture","model":"model"},"small":{"provider":"fixture","model":"model"}},"agents":{"task":{"model":"small","allowed_tools":["view","write","bash"],"allowed_mcp":{"unsafe":["*"]}},"reviewer":{"model":"small","allowed_tools":["view"]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "floyd.json"), []byte(raw), 0o600))
	store, err := Init(dir, "", false)
	require.NoError(t, err)
	for range 2 {
		store.SetupAgents()
		task := store.Config().Agents[AgentTask]
		require.Equal(t, SelectedModelTypeSmall, task.Model)
		require.Equal(t, []string{"view"}, task.AllowedTools)
		require.Empty(t, task.AllowedMCP)
		reviewer := store.Config().Agents["reviewer"]
		require.Equal(t, "reviewer", reviewer.ID)
		require.Equal(t, SelectedModelTypeSmall, reviewer.Model)
		require.Equal(t, []string{"view"}, reviewer.AllowedTools)
	}
	store.Config().Options.DisabledTools = []string{"view"}
	store.SetupAgents()
	require.Empty(t, store.Config().Agents[AgentTask].AllowedTools)
	store.Config().Options.DisabledTools = nil
	store.SetupAgents()
	require.Equal(t, []string{"view"}, store.Config().Agents[AgentTask].AllowedTools, "rebuilding defaults must preserve original explicit choices")
	require.NoError(t, store.ReloadFromDisk(t.Context()))
	require.Equal(t, SelectedModelTypeSmall, store.Config().Agents[AgentTask].Model)
}
