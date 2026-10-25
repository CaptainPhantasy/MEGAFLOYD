package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/stretchr/testify/require"
)

func TestModernHooksCoordinatorChildrenAndDiagnostics(t *testing.T) {
	env := testEnv(t)
	dataDir := t.TempDir()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	defer db.Release(dataDir)
	cfg := &config.Config{Options: &config.Options{DataDirectory: dataDir, Attribution: &config.Attribution{TrailerStyle: config.TrailerStyleNone}}, Hooks: map[string][]config.HookConfig{
		hooks.EventPreToolUse:     {{Name: "deny-action", Command: `echo '{"decision":"deny","reason":"fixture-denied"}'`, Matcher: ".*"}},
		hooks.EventActionRejected: {{Name: "rejection-observer", Command: `echo '{}'`}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	store := config.NewTestStoreWithWorkingDir(cfg, env.workingDir)
	runtime := hookruntime.New(t.Context(), store, hookstore.New(conn))
	defer runtime.Close(context.Background())
	c := &coordinator{cfg: store, sessions: env.sessions, messages: env.messages, permissions: env.permissions, history: env.history, filetracker: *env.filetracker, hookRuntime: runtime}
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "child-session")
	ctx = hookruntime.WithParent(hookruntime.WithRun(ctx, "child-run"), "parent-call")
	inner := &fakeTool{name: "write", resp: fantasy.NewTextResponse("effect")}
	wrapped := c.wrapAgentTools([]fantasy.AgentTool{inner}, true)
	response, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: "child-call", Name: "write", Input: `{"file_path":"test.txt","content":"x"}`})
	require.NoError(t, err)
	require.True(t, response.IsError)
	require.False(t, inner.called)
	page, err := runtime.Store().List(t.Context(), hookstore.Query{SessionID: "child-session", ToolCallID: "child-call"})
	require.NoError(t, err)
	require.Len(t, page.Records, 4)
	require.Equal(t, hooks.EventActionRejected, page.Records[3].Event)
	require.Equal(t, "parent-call", page.Records[3].ParentToolCallID)
	require.Equal(t, "child-run", page.Records[3].RunID)

	// Obtain the diagnostic tools from the real palette, through normal filters.
	diag, err := c.buildTools(t.Context(), config.Agent{AllowedTools: []string{tools.HookExplainToolName, tools.HookTraceToolName}}, false)
	require.NoError(t, err)
	require.Len(t, diag, 2)
	for _, tool := range diag {
		input := `{}`
		if tool.Info().Name == tools.HookExplainToolName {
			input = `{"event":"PreToolUse","tool_name":"bash"}`
		}
		response, err := tool.Run(ctx, fantasy.ToolCall{ID: "diagnostic-" + tool.Info().Name, Name: tool.Info().Name, Input: input})
		require.NoError(t, err)
		require.False(t, response.IsError, response.Content)
	}
	after, err := runtime.Store().List(t.Context(), hookstore.Query{})
	require.NoError(t, err)
	require.Len(t, after.Records, 4, "diagnostics must not execute broad hook matchers")

	// The outer observation sees the real plan guard before the pre hook.
	_, err = c.planService().Enter("plan-session")
	require.NoError(t, err)
	planCtx := context.WithValue(t.Context(), tools.SessionIDContextKey, "plan-session")
	guarded := c.wrapAgentTools([]fantasy.AgentTool{inner}, false)
	response, err = guarded[0].Run(planCtx, fantasy.ToolCall{ID: "plan-call", Name: "write", Input: `{"file_path":"source.go","content":"package main"}`})
	require.NoError(t, err)
	require.True(t, response.IsError)
	page, err = runtime.Store().List(t.Context(), hookstore.Query{ToolCallID: "plan-call"})
	require.NoError(t, err)
	require.Len(t, page.Records, 2)
	require.Equal(t, hooks.EventActionRejected, page.Records[1].Event)
}

func TestModernHooksNativeSessionLifecycle(t *testing.T) {
	env := testEnv(t)
	dataDir := t.TempDir()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	defer db.Release(dataDir)
	cfg := &config.Config{Options: &config.Options{DataDirectory: dataDir}, Hooks: map[string][]config.HookConfig{
		hooks.EventSessionStart: {{Command: `echo '{"context":"hook-start-context"}'`}},
		hooks.EventStop:         {{Command: `echo '{}'`}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, env.workingDir), hookstore.New(conn))
	defer runtime.Close(context.Background())
	sess, err := env.sessions.Create(t.Context(), "child lifecycle")
	require.NoError(t, err)
	// Existing assistant context still counts as the first real user run.
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "prior context"}}})
	require.NoError(t, err)
	model := &finishStreamModel{text: "done"}
	a := testSessionAgent(env, model, model, "base").(*sessionAgent)
	a.isSubAgent = true
	a.hookRuntime = runtime
	_, err = a.Run(hookruntime.WithParent(t.Context(), "parent-task"), SessionAgentCall{SessionID: sess.ID, RunID: "child-turn-1", Prompt: "first"})
	require.NoError(t, err)
	_, err = a.Run(hookruntime.WithParent(t.Context(), "parent-task"), SessionAgentCall{SessionID: sess.ID, RunID: "child-turn-2", Prompt: "second"})
	require.NoError(t, err)
	records, err := runtime.Store().List(t.Context(), hookstore.Query{SessionID: sess.ID})
	require.NoError(t, err)
	counts := map[string]int{}
	for _, record := range records.Records {
		if record.State == "succeeded" {
			counts[record.Event]++
			require.Equal(t, "parent-task", record.ParentToolCallID)
		}
	}
	require.Equal(t, 1, counts[hooks.EventSessionStart])
	require.Equal(t, 2, counts[hooks.EventStop])
	msgs, err := env.messages.List(t.Context(), sess.ID)
	require.NoError(t, err)
	found := false
	for _, msg := range msgs {
		if msg.Role == message.User && msg.Content().String() == "hook-start-context\n\nfirst" {
			found = true
		}
	}
	require.True(t, found, "SessionStart context must reach the actual native user prompt")
}

func TestModernHooksPostRequiredEffectAndRewrittenInput(t *testing.T) {
	env := testEnv(t)
	cfg := &config.Config{Options: &config.Options{DataDirectory: t.TempDir()}, Hooks: map[string][]config.HookConfig{
		hooks.EventPreToolUse:  {{Command: `echo '{"updated_input":{"command":"rewritten"}}'`}},
		hooks.EventPostToolUse: {{Command: `cat > post.json; exit 1`, Required: true}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, env.workingDir), nil)
	defer runtime.Close(context.Background())
	ran := false
	response, err := runtime.ExecuteTool(t.Context(), "session", fantasy.ToolCall{ID: "call", Name: "bash", Input: `{"command":"original"}`}, func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		require.JSONEq(t, `{"command":"rewritten"}`, call.Input)
		ran = true
		return fantasy.NewTextResponse("effect happened"), nil
	})
	require.NoError(t, err)
	require.True(t, ran)
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "effects have already occurred")
	body, err := os.ReadFile(filepath.Join(env.workingDir, "post.json"))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "rewritten", payload["tool_input"].(map[string]any)["command"])
}

func TestModernHooksCancelledPreDoesNotEnterBody(t *testing.T) {
	cfg := &config.Config{Options: &config.Options{DataDirectory: t.TempDir()}, Hooks: map[string][]config.HookConfig{
		hooks.EventPreToolUse: {{Command: `sleep 5; echo '{}'`}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, t.TempDir()), nil)
	defer runtime.Close(context.Background())
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	entered := false
	response, err := runtime.ExecuteTool(ctx, "session", fantasy.ToolCall{ID: "cancel-before-body", Name: "write", Input: `{}`}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		entered = true
		return fantasy.NewTextResponse("wrote"), nil
	})
	require.NoError(t, err)
	require.False(t, entered, "cancelled action must not enter an unstarted tool body")
	require.True(t, response.IsError)
}
