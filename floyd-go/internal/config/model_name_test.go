package config

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestResolveModelNamesAndAmbiguity(t *testing.T) {
	t.Parallel()
	cfg := &Config{Providers: csync.NewMap[string, ProviderConfig]()}
	cfg.Providers.Set("one", ProviderConfig{Models: []catwalk.Model{{ID: "claude-sonnet-4", Name: "Claude Sonnet"}, {ID: "gpt/test", Name: "Unique"}}})
	cfg.Providers.Set("two", ProviderConfig{Models: []catwalk.Model{{ID: "claude-sonnet-4", Name: "Claude Sonnet"}}})
	for _, input := range []string{"one/claude-sonnet-4", "ONE/Claude Sonnet", "one/sonnet"} {
		model, err := ResolveModel(cfg, input)
		require.NoError(t, err, input)
		require.Equal(t, SelectedModel{Provider: "one", Model: "claude-sonnet-4"}, model)
	}
	model, err := ResolveModel(cfg, "gpt/test")
	require.NoError(t, err)
	require.Equal(t, "gpt/test", model.Model)
	_, err = ResolveModel(cfg, "sonnet")
	require.ErrorContains(t, err, "ambiguous")
	_, err = ResolveModel(cfg, "absent")
	require.ErrorContains(t, err, "no configured model")
}
