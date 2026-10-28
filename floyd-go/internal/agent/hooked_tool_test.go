package agent

import (
	"context"
	"os"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

// fakeTool records the context it was invoked with so tests can assert on
// values stamped onto it by the hookedTool decorator.
type fakeTool struct {
	name   string
	called bool
	gotCtx context.Context
	resp   fantasy.ToolResponse
}

func (f *fakeTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: f.name}
}

func (f *fakeTool) Run(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	f.called = true
	f.gotCtx = ctx
	return f.resp, nil
}

func (f *fakeTool) ProviderOptions() fantasy.ProviderOptions     { return nil }
func (f *fakeTool) SetProviderOptions(_ fantasy.ProviderOptions) {}

// newRunner builds a hooks.Runner from a single HookConfig, running the
// config-loader path that compiles the matcher regex.
func newRunner(t *testing.T, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			hooks.EventPreToolUse: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[hooks.EventPreToolUse], t.TempDir(), t.TempDir())
}

func TestHookedTool_AllowStampsHookApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	runner := newRunner(t, `echo '{"decision":"allow"}'`)
	tool := newHookedTool(inner, runner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called, "inner tool should have run")

	// The inner tool's permission service can now treat call-1 as pre-approved.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	granted, err := svc.Request(inner.gotCtx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-1",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.NoError(t, err)
	require.True(t, granted, "hook allow should bypass the permission prompt")
}

func TestHookedTool_SilentDoesNotStampApproval(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("ok")}
	runner := newRunner(t, `exit 0`) // no stdout, no decision
	tool := newHookedTool(inner, runner, nil)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-2", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called)

	// With no hook opinion, a fresh permission request has nothing stamped
	// and must fall through to the normal flow. We verify by checking that
	// the context does not look pre-approved for this call ID: sending a
	// request that no subscriber resolves will block until cancelled.
	svc := permission.NewPermissionService(t.TempDir(), false, nil)
	ctx, cancel := context.WithCancel(inner.gotCtx)
	cancel()
	granted, err := svc.Request(ctx, permission.CreatePermissionRequest{
		SessionID:  "s1",
		ToolCallID: "call-2",
		ToolName:   "view",
		Action:     "read",
		Path:       t.TempDir(),
	})
	require.Error(t, err, "no approval stamped => request should reach the prompt path")
	require.False(t, granted)
}

func TestHookedTool_DenySkipsInnerTool(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash"}
	runner := newRunner(t, `echo "blocked" >&2; exit 2`)
	tool := newHookedTool(inner, runner, nil)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-3", Name: "bash"})
	require.NoError(t, err)
	require.False(t, inner.called, "denied call must not reach the inner tool")
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "blocked")
}

// TestRuntimeWrappersCoverEveryTool guards the live tool-dispatch path: the
// coordinator and the agentic-fetch tool wrap with the runtime variants, so
// every non-diagnostic tool must be wrapped for both hooks phases while the
// hook diagnostics stay read-only.
func TestRuntimeWrappersCoverEveryTool(t *testing.T) {
	t.Parallel()

	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(&config.Config{Hooks: map[string][]config.HookConfig{}}, t.TempDir()), nil)
	defer runtime.Close(context.Background())

	inputs := []fantasy.AgentTool{
		&fakeTool{name: "view"},
		&fakeTool{name: tools.HookExplainToolName},
		&fakeTool{name: tools.HookTraceToolName},
	}

	pre := wrapToolsWithRuntimePre(inputs, runtime)
	require.Len(t, pre, len(inputs))
	_, isPre := pre[0].(*runtimePreTool)
	require.True(t, isPre, "an ordinary tool must be wrapped for PreToolUse")
	require.Equal(t, inputs[1], pre[1], "hook diagnostics stay read-only")
	require.Equal(t, inputs[2], pre[2], "hook diagnostics stay read-only")

	observed := wrapToolsWithRuntimeObservation(inputs, runtime)
	require.Len(t, observed, len(inputs))
	_, isObserved := observed[0].(*runtimeObservedTool)
	require.True(t, isObserved, "an ordinary tool must be observed after the call")
	require.Equal(t, inputs[1], observed[1], "hook diagnostics stay read-only")
	require.Equal(t, inputs[2], observed[2], "hook diagnostics stay read-only")
}

// newEventRunner builds a hooks.Runner for an arbitrary event, running
// the same validation path the config loader uses.
func newEventRunner(t *testing.T, event, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			event: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[event], t.TempDir(), t.TempDir())
}

func TestHookedTool_PostToolUseAppendsContext(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("tool output")}
	runner := newEventRunner(t, hooks.EventPostToolUse, `echo '{"context":"post hook saw it"}'`)
	tool := newHookedTool(inner, nil, runner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view", Input: `{"x":1}`})
	require.NoError(t, err)
	require.True(t, inner.called)
	require.Contains(t, resp.Content, "tool output")
	require.Contains(t, resp.Content, "post hook saw it")
}

func TestHookedTool_PostToolUseDenyIsAdvisoryOnly(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("tool output")}
	runner := newEventRunner(t, hooks.EventPostToolUse, `echo '{"decision":"deny","reason":"late denial"}'`)
	tool := newHookedTool(inner, nil, runner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called, "post tool deny cannot unrun the tool")
	require.Contains(t, resp.Content, "tool output")
	require.NotContains(t, resp.Content, "late denial", "advisory deny must not surface as block text")
}

func TestHookedTool_PostToolUseReceivesToolResponse(t *testing.T) {
	t.Parallel()

	// The hook writes the tool_response payload it received to a file so
	// the test can assert the tool's output reached the hook.
	out := t.TempDir() + "/payload.json"
	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("stdout line")}
	runner := newEventRunner(t, hooks.EventPostToolUse,
		`tee `+out+` >/dev/null; echo '{"decision":"none"}'`)
	tool := newHookedTool(inner, nil, runner)

	_, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "bash", Input: `{"command":"ls"}`})
	require.NoError(t, err)
	data, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.Contains(t, string(data), `"event":"PostToolUse"`)
	require.Contains(t, string(data), `"tool_name":"bash"`)
	require.Contains(t, string(data), "stdout line", "tool response must be in the hook payload")
}

func TestHookedTool_PostToolUseErrorDoesNotFailToolCall(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("tool output")}
	runner := newEventRunner(t, hooks.EventPostToolUse, `exit 7`)
	tool := newHookedTool(inner, nil, runner)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: "view"})
	require.NoError(t, err, "post-hook failure must not fail the already-executed tool")
	require.Contains(t, resp.Content, "tool output")
}
