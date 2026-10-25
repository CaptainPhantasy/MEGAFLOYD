package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestCanonicalModelSwitchUsesOwnDefaults(t *testing.T) {
	dir := t.TempDir()
	beam := SelectedModel{Provider: "opencode-go", Model: "beam-free", MaxTokens: 131072, ReasoningEffort: "medium", ProviderOptions: map[string]any{"reasoning_effort": "medium", "max_tokens": float64(131072)}}
	providers := csync.NewMap[string, ProviderConfig]()
	providers.Set("opencode-go", ProviderConfig{ID: "opencode-go", Models: []catwalk.Model{{ID: "beam-free", DefaultMaxTokens: 131072, CanReason: true, ReasoningLevels: []string{"low", "medium", "high"}}}})
	providers.Set("target", ProviderConfig{ID: "target", Models: []catwalk.Model{
		{ID: "plain", DefaultMaxTokens: 8192},
		{ID: "reasoning", DefaultMaxTokens: 32768, CanReason: true, ReasoningLevels: []string{"low", "medium", "high"}, DefaultReasoningEffort: "medium"},
		{ID: "bounded", DefaultMaxTokens: 4096, CanReason: true, ReasoningLevels: []string{"low", "medium"}},
		{ID: "boolean", DefaultMaxTokens: 16384, CanReason: true},
	}})
	cfg := &Config{Providers: providers, Models: map[SelectedModelType]SelectedModel{SelectedModelTypeLarge: beam}}
	store := NewTestStoreWithWorkingDir(cfg, dir)
	store.globalDataPath = filepath.Join(dir, "global.json")
	store.workspacePath = filepath.Join(dir, ".megafloyd", "floyd.json")
	originalJSON, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.globalDataPath, originalJSON, 0o600))
	require.NoError(t, store.UpdatePreferredModel(ScopeGlobal, SelectedModelTypeLarge, beam))
	require.Equal(t, beam, store.Config().Models[SelectedModelTypeLarge])
	globalBefore, err := os.ReadFile(store.globalDataPath)
	require.NoError(t, err)
	// Reproduce the old web request, including stale Beam values.
	stale := beam
	stale.Provider = "target"
	stale.Model = "plain"
	stale.Think = true
	stale.ProviderOptions = map[string]any{"reasoning_effort": "medium", "max_tokens": 131072}
	require.NoError(t, store.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, stale))
	got := store.Config().Models[SelectedModelTypeLarge]
	require.Nil(t, got.ProviderOptions)
	require.Equal(t, int64(8192), got.MaxTokens)
	require.Empty(t, got.ReasoningEffort)
	require.False(t, got.Think)
	saved := readConfigJSON(t, store.workspacePath)["models"].(map[string]any)["large"].(map[string]any)
	require.Equal(t, float64(8192), saved["max_tokens"])
	require.Equal(t, "", saved["reasoning_effort"], "workspace reset must override the global medium field")
	require.Equal(t, false, saved["think"])
	require.Contains(t, saved, "provider_options")
	require.Nil(t, saved["provider_options"])
	reloaded, _, err := loadFromConfigPaths(context.Background(), []string{store.globalDataPath, store.workspacePath})
	require.NoError(t, err)
	require.Equal(t, got, reloaded.Models[SelectedModelTypeLarge], "real global+workspace loader must preserve the new model resets")
	initial := &Config{Providers: providers, Models: map[SelectedModelType]SelectedModel{SelectedModelTypeLarge: {Provider: "target", Model: "reasoning"}, SelectedModelTypeSmall: {Provider: "target", Model: "boolean"}}}
	resolved, err := resolveSelectedModels(initial, nil)
	require.NoError(t, err)
	require.Equal(t, "high", resolved.Large.ReasoningEffort)
	require.Equal(t, int64(32768), resolved.Large.MaxTokens)
	require.Empty(t, resolved.Small.ReasoningEffort)
	require.True(t, resolved.Small.Think)

	globalAfter, err := os.ReadFile(store.globalDataPath)
	require.NoError(t, err)
	require.Equal(t, globalBefore, globalAfter)
	for _, tc := range []struct {
		id     string
		tokens int64
		effort string
		think  bool
	}{
		{"reasoning", 32768, "high", false}, {"bounded", 4096, "medium", false}, {"boolean", 16384, "", true},
	} {
		stale.Model = tc.id
		require.NoError(t, store.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, stale))
		got = store.Config().Models[SelectedModelTypeLarge]
		require.Equal(t, tc.tokens, got.MaxTokens)
		require.Equal(t, tc.effort, got.ReasoningEffort)
		require.Equal(t, tc.think, got.Think)
	}

	// The terminal's boolean toggle changes Think only. Its explicit off must
	// survive reload instead of becoming the initial enabled default again.
	for _, enabled := range []bool{false, true} {
		toggle := store.Config().Models[SelectedModelTypeLarge]
		toggle.Think = enabled
		require.NoError(t, store.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, toggle))
		reloaded, _, err = loadFromConfigPaths(context.Background(), []string{store.globalDataPath, store.workspacePath})
		require.NoError(t, err)
		reloadedSelection, err := resolveSelectedModels(reloaded, nil)
		require.NoError(t, err)
		require.Equal(t, enabled, reloadedSelection.Large.Think)
		if enabled {
			require.Empty(t, reloadedSelection.Large.ReasoningEffort)
		} else {
			require.Equal(t, "none", reloadedSelection.Large.ReasoningEffort)
		}
	}
	require.NoError(t, store.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, SelectedModel{Provider: "target", Model: "reasoning"}))
	explicit := store.Config().Models[SelectedModelTypeLarge]
	explicit.ReasoningEffort = "low"
	explicit.MaxTokens = 2048
	require.NoError(t, store.UpdatePreferredModel(ScopeWorkspace, SelectedModelTypeLarge, explicit))
	require.Equal(t, explicit, store.Config().Models[SelectedModelTypeLarge], "same-model explicit choices remain usable")
}
