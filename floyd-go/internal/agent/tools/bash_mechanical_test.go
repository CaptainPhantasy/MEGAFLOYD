package tools

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/stretchr/testify/require"
)

func TestMechanicalBashFailureHasNativeReceipt(t *testing.T) {
	cwd := t.TempDir()
	data := t.TempDir()
	conn, err := db.Connect(t.Context(), data)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Release(data) })
	store := hookstore.New(conn)
	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(&config.Config{}, cwd), store)
	defer runtime.Close(context.Background())
	tool := newBashToolForTest(cwd)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "mechanical-bash")
	call := fantasy.ToolCall{ID: "native-bash-exit7", Name: BashToolName, Input: `{"description":"verify nonzero result","command":"exit 7"}`}
	response, err := runtime.ObserveTool(ctx, "mechanical-bash", call, func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return runtime.ExecuteRegisteredBody(ctx, tool.Info(), call, tool.Run)
	})
	require.NoError(t, err)
	var metadata BashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(response.Metadata), &metadata))
	require.NotNil(t, metadata.ExitCode)
	require.Equal(t, 7, *metadata.ExitCode)
	page, err := store.List(t.Context(), hookstore.Query{SessionID: "mechanical-bash", ToolCallID: call.ID})
	require.NoError(t, err)
	require.Len(t, page.Records, 2)
	require.Equal(t, "builtin", page.Records[0].Mode)
	require.Equal(t, "registered-tool-schema", page.Records[0].HookName)
	require.Equal(t, hooks.EventPostToolUseFailure, page.Records[1].Event)
	require.Equal(t, "failed", page.Records[1].Outcome)
	require.Equal(t, 7, *page.Records[1].ExitCode)
	require.Empty(t, page.Records[1].CommandHash)
	// Explicit native session-shell shares validation and retains safe receipts.
	ran := false
	response, err = runtime.ExecuteTool(ctx, "mechanical-bash", fantasy.ToolCall{ID: "invalid-direct-shell", Name: "bash", Input: `{"command":7}`}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ran = true
		return fantasy.NewTextResponse("ran"), nil
	})
	require.NoError(t, err)
	require.False(t, ran)
	require.True(t, response.IsError)
}
