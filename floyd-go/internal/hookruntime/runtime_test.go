package hookruntime

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/stretchr/testify/require"
)

func TestNativeToolLifecycleRecordsProvenSuccessFailureAndSchemaRejection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, input string
		response    fantasy.ToolResponse
		expected    string
		run         bool
	}{
		{"success", `{"command":"printf fixture"}`, fantasy.NewTextResponse("fixture"), hooks.EventPostToolUse, true},
		{"failed", `{"command":"exit 7"}`, fantasy.ToolResponse{Metadata: `{"exit_code":7}`}, hooks.EventPostToolUseFailure, true},
		{"invalid input", `{"command":7}`, fantasy.ToolResponse{}, hooks.EventActionRejected, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			conn, err := db.Connect(t.Context(), dir)
			require.NoError(t, err)
			defer db.Release(dir)
			runtime := New(t.Context(), config.NewTestStoreWithWorkingDir(&config.Config{}, t.TempDir()), hookstore.New(conn))
			defer runtime.Close(context.Background())
			ran := false
			ctx := WithParent(WithRun(t.Context(), "fixture-run"), "fixture-parent")
			response, err := runtime.ExecuteTool(ctx, "fixture-session", fantasy.ToolCall{ID: "call", Name: "bash", Input: tc.input}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				ran = true
				return tc.response, nil
			})
			require.NoError(t, err)
			require.Equal(t, tc.run, ran)
			if !tc.run {
				require.True(t, response.IsError)
			}
			page, err := runtime.Store().List(t.Context(), hookstore.Query{ToolCallID: "call"})
			require.NoError(t, err)
			require.NotEmpty(t, page.Records)
			last := page.Records[len(page.Records)-1]
			if tc.run {
				require.Equal(t, tc.expected, last.Event)
			} else {
				require.Equal(t, "denied", last.State)
			}
			require.Equal(t, "fixture-run", last.RunID)
			require.Equal(t, "fixture-parent", last.ParentToolCallID)
			for _, record := range page.Records {
				raw, err := json.Marshal(record)
				require.NoError(t, err)
				require.NotContains(t, string(raw), "printf fixture")
			}
		})
	}
}

func TestDenyHookPreventsToolBodyAndRetainsCorrelation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	defer db.Release(dir)
	cfg := &config.Config{Hooks: map[string][]config.HookConfig{
		hooks.EventPreToolUse:     {{Name: "deny-fixture", Command: `printf '%s' '{"decision":"deny","reason":"fixture refused"}'`}},
		hooks.EventActionRejected: {{Name: "observe-rejection", Command: `printf '{}'`}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	runtime := New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, t.TempDir()), hookstore.New(conn))
	defer runtime.Close(context.Background())
	ran := false
	response, err := runtime.ExecuteTool(WithRun(t.Context(), "run1"), "s1", fantasy.ToolCall{ID: "call1", Name: "bash", Input: `{"command":"printf fixture"}`}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ran = true
		return fantasy.ToolResponse{}, nil
	})
	require.NoError(t, err)
	require.True(t, response.IsError)
	require.False(t, ran)
	require.Contains(t, response.Content, "fixture refused")
	page, err := runtime.Store().List(t.Context(), hookstore.Query{ToolCallID: "call1"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Records)
	require.Equal(t, hooks.EventActionRejected, page.Records[len(page.Records)-1].Event)
	require.Equal(t, "run1", page.Records[len(page.Records)-1].RunID)
	require.NoError(t, runtime.Close(t.Context()))
	result, err := runtime.Run(t.Context(), hooks.EventPreToolUse, "s1", "bash", `{}`, hooks.EventDetails{})
	require.NoError(t, err)
	require.Zero(t, result.HookCount)
}
