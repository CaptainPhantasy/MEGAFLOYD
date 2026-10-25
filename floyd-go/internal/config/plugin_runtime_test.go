package config

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPluginSnapshotPersistence(t *testing.T) {
	dir := t.TempDir()
	s := NewTestStoreWithWorkingDir(&Config{Options: &Options{}}, dir)
	s.workspacePath = filepath.Join(dir, ".megafloyd", "megafloyd.json")
	err := s.UpdatePluginRuntime(func(cfg *Config) (map[string]any, error) {
		cfg.Options.PluginRuntime = map[string]any{"system_prompts": []any{map[string]any{"pluginId": "fixture", "content": "persisted instructions"}}}
		return map[string]any{"options.plugin_runtime": cfg.Options.PluginRuntime}, nil
	})
	require.NoError(t, err)
	data, err := os.ReadFile(s.workspacePath)
	require.NoError(t, err)
	var restored Config
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, s.Config().Options.PluginRuntime, restored.Options.PluginRuntime)
	block := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(block, []byte("occupied"), 0600))
	s.workspacePath = filepath.Join(block, "config.json")
	err = s.UpdatePluginRuntime(func(cfg *Config) (map[string]any, error) {
		cfg.Options.PluginRuntime = map[string]any{"changed": true}
		return map[string]any{"options.plugin_runtime": cfg.Options.PluginRuntime}, nil
	})
	require.Error(t, err)
	require.NotContains(t, s.Config().Options.PluginRuntime, "changed")
}
