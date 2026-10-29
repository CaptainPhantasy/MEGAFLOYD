package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/stretchr/testify/require"
)

func TestHookExplainUsesEnginePlanWithoutExecution(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "must-not-run")
	command := "touch '" + marker + "'"
	cfg := config.NewTestStore(&config.Config{Hooks: map[string][]config.HookConfig{
		"PreToolUse": {
			{Name: "first", Matcher: "^bash$", Command: command},
			{Matcher: "^view$", Command: "echo private-command-token"},
			{Name: "duplicate", Matcher: "^bash$", Command: command},
			{Name: "ordered check", Command: "echo second", Sequential: true, Required: true},
		},
	}})
	response := runHookInspection(t, NewHookExplainTool(cfg), context.Background(), HookExplainParams{Event: "pre_tool_use", ToolName: "bash"})
	require.False(t, response.IsError, response.Content)
	var explanation HookExplanation
	require.NoError(t, json.Unmarshal([]byte(response.Content), &explanation))
	require.True(t, explanation.InspectionOnly)
	require.Equal(t, "effective_runtime", explanation.Source)
	require.Equal(t, "PreToolUse", explanation.Event)
	require.Len(t, explanation.Entries, 4)
	require.Equal(t, "hooks.PreToolUse[0]", explanation.Entries[0].ConfigKey)
	require.Equal(t, 1, explanation.Entries[0].Order)
	require.False(t, explanation.Entries[1].Matched)
	require.Equal(t, 0, explanation.Entries[1].Order)
	require.NotNil(t, explanation.Entries[2].DuplicateOf)
	require.Equal(t, 0, *explanation.Entries[2].DuplicateOf)
	require.Equal(t, 0, explanation.Entries[2].Order)
	require.Equal(t, 2, explanation.Entries[3].Order)
	require.Equal(t, "ordered", explanation.Entries[3].Mode)
	require.True(t, explanation.Entries[3].CanBlock)
	require.True(t, explanation.Entries[3].CanApprove)
	require.True(t, explanation.Entries[3].CanRewrite)
	require.False(t, explanation.Entries[3].Observational)
	require.Len(t, explanation.Entries[0].CommandHash, 64)
	require.NotContains(t, response.Content+response.Metadata, command)
	require.NotContains(t, response.Content+response.Metadata, "private-command-token")
	_, err := os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHookExplainReportsObserverAndRejectsUnknownEvent(t *testing.T) {
	t.Parallel()
	cfg := config.NewTestStore(&config.Config{Hooks: map[string][]config.HookConfig{
		"PostToolUse": {{Command: "echo observe", Async: true}},
	}})
	response := runHookInspection(t, NewHookExplainTool(cfg), context.Background(), HookExplainParams{Event: "PostToolUse", ToolName: "bash"})
	var explanation HookExplanation
	require.NoError(t, json.Unmarshal([]byte(response.Content), &explanation))
	require.Len(t, explanation.Entries, 1)
	require.True(t, explanation.Entries[0].Observational)
	require.False(t, explanation.Entries[0].CanBlock)
	require.False(t, explanation.Entries[0].CanApprove)
	require.False(t, explanation.Entries[0].CanRewrite)
	require.Equal(t, "async", explanation.Entries[0].Mode)
	response = runHookInspection(t, NewHookExplainTool(cfg), context.Background(), HookExplainParams{Event: "InventedEvent"})
	require.True(t, response.IsError)
}

func TestHookTraceReadsDurableScopedHistory(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "session-a")
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := hookstore.New(conn)
	for i, state := range []string{"accepted", "running", "completed"} {
		require.NoError(t, store.Append(ctx, hookstore.Record{
			ID: string(rune('a' + i)), EventID: "event-a", Event: "PostToolUse",
			JobID: "job-a", SessionID: "session-a", ToolCallID: "call-a", State: state,
		}))
	}
	require.NoError(t, store.Append(ctx, hookstore.Record{
		ID: "other-session", EventID: "event-b", Event: "PreToolUse",
		SessionID: "session-b", ToolCallID: "call-b", State: "completed",
	}))
	tool := NewHookTraceTool(store)
	first := decodeHookPage(t, runHookInspection(t, tool, ctx, HookTraceParams{Limit: 2}))
	require.Len(t, first.Records, 2)
	require.True(t, first.HasMore)
	require.Greater(t, first.Next, int64(0))
	last := decodeHookPage(t, runHookInspection(t, tool, ctx, HookTraceParams{After: first.Next, Limit: 2}))
	require.Len(t, last.Records, 1)
	require.Equal(t, "completed", last.Records[0].State)
	require.False(t, last.HasMore)
	filtered := decodeHookPage(t, runHookInspection(t, tool, ctx, HookTraceParams{ToolCallID: "call-b"}))
	require.Empty(t, filtered.Records)
	filtered = decodeHookPage(t, runHookInspection(t, tool, ctx, HookTraceParams{EventID: "event-a", JobID: "job-a"}))
	require.Len(t, filtered.Records, 3)
	require.NoError(t, db.Release(dataDir))
	reader, closer, err := hookstore.OpenReader(dataDir)
	require.NoError(t, err)
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}
	reopened := decodeHookPage(t, runHookInspection(t, NewHookTraceTool(reader), ctx, HookTraceParams{}))
	require.Len(t, reopened.Records, 3)
	require.Equal(t, filtered.Records, reopened.Records)
	other, otherCloser, err := hookstore.OpenReader(t.TempDir())
	require.NoError(t, err)
	if otherCloser != nil {
		t.Cleanup(func() { _ = otherCloser.Close() })
	}
	require.Empty(t, decodeHookPage(t, runHookInspection(t, NewHookTraceTool(other), ctx, HookTraceParams{})).Records)
}

func TestHookTraceReportsReadErrorsAndInvalidCursor(t *testing.T) {
	t.Parallel()
	response := runHookInspection(t, NewHookTraceTool((*hookstore.Store)(nil)), context.Background(), HookTraceParams{})
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "unavailable")
	reader := &failingHookReader{}
	response = runHookInspection(t, NewHookTraceTool(reader), context.Background(), HookTraceParams{After: -1})
	require.True(t, response.IsError)
	require.False(t, reader.called)
	response = runHookInspection(t, NewHookTraceTool(reader), context.Background(), HookTraceParams{})
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "storage unavailable")
	require.True(t, reader.called)
}

type failingHookReader struct{ called bool }

func (r *failingHookReader) List(context.Context, hookstore.Query) (hookstore.Page, error) {
	r.called = true
	return hookstore.Page{}, errors.New("storage unavailable")
}

func runHookInspection(t *testing.T, tool fantasy.AgentTool, ctx context.Context, params any) fantasy.ToolResponse {
	t.Helper()
	data, err := json.Marshal(params)
	require.NoError(t, err)
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "inspection", Name: tool.Info().Name, Input: string(data)})
	require.NoError(t, err)
	return response
}

func decodeHookPage(t *testing.T, response fantasy.ToolResponse) hookstore.Page {
	t.Helper()
	require.False(t, response.IsError, response.Content)
	var page hookstore.Page
	require.NoError(t, json.Unmarshal([]byte(response.Content), &page))
	return page
}

func TestHookTraceFollowsActualDispatch(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "session-a")
	ctx = hookruntime.WithParent(hookruntime.WithRun(ctx, "run-a"), "parent-a")
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Release(dataDir) })
	store := hookstore.New(conn)
	observer := config.HookConfig{Command: `printf '{"decision":"allow"}'`}
	async := observer
	async.Async = true
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{Hooks: map[string][]config.HookConfig{
		hooks.EventPreToolUse:         {{Matcher: "^denied$", Command: "exit 2", Required: true}},
		hooks.EventPostToolUse:        {async},
		hooks.EventPostToolUseFailure: {observer},
		hooks.EventActionRejected:     {observer},
		hooks.EventToolCancelled:      {observer},
	}}, t.TempDir())
	runtime := hookruntime.New(ctx, cfg, store)
	t.Cleanup(func() {
		wait, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, runtime.Close(wait))
	})
	trace := NewHookTraceTool(store)
	cases := []struct {
		name  string
		event string
	}{
		{"success", hooks.EventPostToolUse},
		{"failed", hooks.EventPostToolUseFailure},
		{"cancelled", hooks.EventToolCancelled},
		{"denied", hooks.EventActionRejected},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			called := false
			_, err := runtime.ExecuteTool(ctx, "session-a", fantasy.ToolCall{ID: "call-" + test.name, Name: test.name, Input: `{"private":"input-secret"}`}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				called = true
				switch test.name {
				case "failed":
					return fantasy.NewTextErrorResponse("output-secret"), nil
				case "cancelled":
					return fantasy.ToolResponse{}, context.Canceled
				default:
					return fantasy.NewTextResponse("output-secret"), nil
				}
			})
			if test.name == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.name != "denied", called)
			var terminal hookstore.Record
			require.Eventually(t, func() bool {
				page, err := store.List(ctx, hookstore.Query{SessionID: "session-a", ToolCallID: "call-" + test.name})
				if err != nil {
					return false
				}
				for _, row := range page.Records {
					if row.Event == test.event && row.FinishedAt != 0 {
						terminal = row
						return true
					}
				}
				return false
			}, 3*time.Second, 10*time.Millisecond)
			response := runHookInspection(t, trace, ctx, HookTraceParams{EventID: terminal.EventID, ToolCallID: "call-" + test.name})
			page := decodeHookPage(t, response)
			require.Len(t, page.Records, 2)
			require.Equal(t, page.Records[0].ID, page.Records[1].ID)
			require.Equal(t, "running", page.Records[0].State)
			require.Equal(t, "succeeded", page.Records[1].State)
			outcomes := map[string]string{"success": "completed", "failed": "failed", "cancelled": "cancelled", "denied": "rejected"}
			require.Equal(t, outcomes[test.name], terminal.Outcome)
			require.Equal(t, "run-a", terminal.RunID)
			require.Equal(t, "parent-a", terminal.ParentToolCallID)
			require.NotContains(t, response.Content+response.Metadata, "input-secret")
			require.NotContains(t, response.Content+response.Metadata, "output-secret")
			if test.name == "success" {
				require.NotEmpty(t, terminal.JobID)
				require.Equal(t, "async", terminal.Mode)
			}
		})
	}
}
