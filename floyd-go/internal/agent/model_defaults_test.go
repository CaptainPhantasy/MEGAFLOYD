package agent

import (
	"encoding/json"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
	"charm.land/fantasy/providers/vercel"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCanonicalNonReasoningProviderOptions(t *testing.T) {
	for _, provider := range []config.ProviderConfig{
		{ID: "hyper", Type: openaicompat.Name}, {ID: "deepseek", Type: openaicompat.Name},
		{ID: "anthropic", Type: anthropic.Name}, {ID: "openai", Type: openai.Name},
		{ID: "google", Type: google.Name}, {ID: "openrouter", Type: openrouter.Name}, {ID: "vercel", Type: vercel.Name},
	} {
		t.Run(provider.ID, func(t *testing.T) {
			model := Model{CatwalkCfg: catwalk.Model{ID: "plain", CanReason: false}, ModelCfg: config.SelectedModel{
				ReasoningEffort: "high", Think: true, ProviderOptions: map[string]any{"reasoning_effort": "high", "thinking": map[string]any{"type": "enabled"}},
			}}
			data, err := json.Marshal(getProviderOptions(model, provider))
			require.NoError(t, err)
			var result any
			require.NoError(t, json.Unmarshal(data, &result))
			var inspect func(any)
			inspect = func(value any) {
				if fields, ok := value.(map[string]any); ok {
					for key, child := range fields {
						if child != nil {
							require.NotContains(t, []string{"reasoning_effort", "thinking", "thinking_config", "effort", "reasoning", "reasoning_summary", "enable_thinking"}, key, string(data))
						}
						inspect(child)
					}
				}
			}
			inspect(result)
		})
	}
	model := Model{CatwalkCfg: catwalk.Model{ID: "reasoning", CanReason: true, DefaultReasoningEffort: "high", ReasoningLevels: []string{"low", "medium", "high"}}, ModelCfg: config.SelectedModel{ReasoningEffort: "none"}}
	require.Empty(t, effectiveReasoningEffort(model), "explicit off cannot fall back to the model default")
	options := getProviderOptions(model, config.ProviderConfig{ID: "example", Type: openaicompat.Name})
	require.Nil(t, options[openaicompat.Name].(*openaicompat.ProviderOptions).ReasoningEffort)
	model.ModelCfg.ReasoningEffort = "high"
	hyperOptions := getProviderOptions(model, config.ProviderConfig{ID: "hyper", Type: openaicompat.Name})[openaicompat.Name].(*openaicompat.ProviderOptions)
	require.NotNil(t, hyperOptions.ReasoningEffort)
	require.Equal(t, true, hyperOptions.ExtraBody["thinking"], "named effort must enable Hyper thinking")
	model.ModelCfg.ReasoningEffort = "none"
	hyperOptions = getProviderOptions(model, config.ProviderConfig{ID: "hyper", Type: openaicompat.Name})[openaicompat.Name].(*openaicompat.ProviderOptions)
	require.Nil(t, hyperOptions.ReasoningEffort)
	require.Equal(t, false, hyperOptions.ExtraBody["thinking"])
	model.CatwalkCfg.ReasoningLevels = nil
	model.ModelCfg.ReasoningEffort = ""
	model.ModelCfg.Think = true
	options = getProviderOptions(model, config.ProviderConfig{ID: "anthropic", Type: anthropic.Name})
	parsed := options[anthropic.Name].(*anthropic.ProviderOptions)
	require.NotNil(t, parsed.Thinking)
	require.Nil(t, parsed.Effort, "boolean-only reasoning must not invent high")
}
