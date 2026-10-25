package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/stretchr/testify/require"
)

func TestMechanicalRegisteredSchemaBeforeEffect(t *testing.T) {
	for _, tc := range []struct {
		name, input, command string
		allowed              bool
	}{
		{"valid", `{"value":7}`, "", true},
		{"wrong-type", `{"value":"seven"}`, "", false},
		{"rewritten-wrong-type", `{"value":7}`, `printf '{"updated_input":{"value":"seven"}}'`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config.Config{Hooks: map[string][]config.HookConfig{}}
			if tc.command != "" {
				cfg.Hooks[hooks.EventPreToolUse] = []config.HookConfig{{Command: tc.command}}
			}
			runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, dir), nil)
			defer runtime.Close(context.Background())
			type params struct {
				Value int `json:"value"`
			}
			marker := filepath.Join(dir, "effect")
			tool := fantasy.NewAgentTool("mechanical_fixture", "fixture", func(ctx context.Context, p params, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				require.Equal(t, 7, p.Value)
				return fantasy.NewTextResponse("ran"), os.WriteFile(marker, []byte("effect"), 0o600)
			})
			wrapped := wrapToolsWithRuntimeObservation(wrapToolsWithRuntimePre([]fantasy.AgentTool{tool}, runtime), runtime)
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "mechanical-session")
			response, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: tc.name, Name: tool.Info().Name, Input: tc.input})
			require.NoError(t, err)
			_, statErr := os.Stat(marker)
			require.Equal(t, tc.allowed, statErr == nil)
			require.Equal(t, !tc.allowed, response.IsError, response.Content)
		})
	}
}
