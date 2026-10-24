package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	mcpclient "github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/permission"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRuntimeRefresh(t *testing.T) {
	t.Run("late_MCP_preserves_native_tools", canonicalLateMCPRefresh)
	t.Run("dynamic_context_belongs_to_actual_run", canonicalDynamicContextRefresh)
}

type canonicalRefreshPermissions struct {
	permission.Service
	allow    bool
	requests int
}

func (p *canonicalRefreshPermissions) Request(context.Context, permission.CreatePermissionRequest) (bool, error) {
	p.requests++
	return p.allow, nil
}

func canonicalLateMCPRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var executions atomic.Int32
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "canonical-late-gateway"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "gateway_probe", Description: "Inspect canonical gateway state"},
		func(context.Context, *mcpsdk.CallToolRequest, struct {
			Query string `json:"query"`
		}) (*mcpsdk.CallToolResult, any, error) {
			executions.Add(1)
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: "canonical gateway reached"},
			}}, nil, nil
		})
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, &mcpsdk.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()

	const name = "canonical-late-gateway"
	sessionless, autoLSP := true, false
	env := testEnv(t)
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{
		Options: &config.Options{
			DataDirectory: t.TempDir(), AutoLSP: &autoLSP,
			Attribution: &config.Attribution{TrailerStyle: config.TrailerStyleNone},
		},
		Agents: map[string]config.Agent{config.AgentCoder: {AllowedTools: []string{"bash", "workflow"}}},
		MCP:    config.MCPs{name: {Type: config.MCPHttp, URL: httpServer.URL, Sessionless: &sessionless}},
	}, env.workingDir)
	permissions := &canonicalRefreshPermissions{Service: env.permissions, allow: true}
	model := &intelligenceModel{}
	a := testSessionAgent(env, model, model, "canonical prompt").(*sessionAgent)
	c := &coordinator{
		cfg: cfg, currentAgent: a, sessions: env.sessions, messages: env.messages,
		permissions: permissions, history: env.history, filetracker: *env.filetracker,
	}

	// The initial agent starts while the configured MCP server is absent.
	initial, err := c.buildTools(ctx, cfg.Config().Agents[config.AgentCoder], false)
	require.NoError(t, err)
	a.SetTools(initial)
	require.Len(t, c.ToolCatalog(), 2)
	native := a.tools.Copy()
	engine := c.workflowEngine()

	// Connecting later updates the registry but not the installed agent list.
	require.NoError(t, mcpclient.InitializeSingle(ctx, name, cfg))
	defer func() { require.NoError(t, mcpclient.DisableSingle(cfg, name)) }()
	require.Len(t, c.ToolCatalog(), 2)
	require.NoError(t, c.RefreshTools(ctx))
	require.Len(t, c.ToolCatalog(), 3)
	find := func(name string) fantasy.AgentTool {
		for _, tool := range a.tools.Copy() {
			if tool.Info().Name == name {
				return tool
			}
		}
		t.Fatalf("tool %q not installed", name)
		return nil
	}
	for _, original := range native {
		require.Same(t, original, find(original.Info().Name), "catalog refresh preserves native instances")
	}
	gatewayName := "mcp_" + name + "_gateway_probe"
	gateway := find(gatewayName)
	require.Contains(t, gateway.Info().Required, "query")
	require.Equal(t, map[string]any{"type": "string"}, gateway.Info().Parameters["query"])

	// The refreshed tool executes through the ordinary permission path.
	callCtx := context.WithValue(ctx, tools.SessionIDContextKey, "canonical-session")
	call := fantasy.ToolCall{ID: "gateway-call", Name: gatewayName, Input: `{"query":"status"}`}
	result, err := gateway.Run(callCtx, call)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, result.Content, "canonical gateway reached")
	require.Equal(t, 1, permissions.requests)
	require.EqualValues(t, 1, executions.Load())

	permissions.allow = false
	result, err = gateway.Run(callCtx, call)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, 2, permissions.requests)
	require.EqualValues(t, 1, executions.Load(), "denied permission never reaches the MCP server")

	// Refreshed MCP wrappers also honor the current hook configuration.
	permissions.allow = true
	cfg.Config().Hooks = map[string][]config.HookConfig{hooks.EventPreToolUse: {{
		Command: `echo '{"decision":"deny","reason":"canonical-hook-denial"}'`,
		Matcher: "^mcp_", Timeout: 2,
	}}}
	require.NoError(t, c.RefreshTools(ctx))
	result, err = find(gatewayName).Run(callCtx, call)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, "canonical-hook-denial")
	require.Equal(t, 2, permissions.requests, "hook denial precedes ordinary tool permission")
	require.EqualValues(t, 1, executions.Load())

	// A full model/config tool rebuild keeps the coordinator's workflow state.
	_, err = c.buildTools(ctx, cfg.Config().Agents[config.AgentCoder], false)
	require.NoError(t, err)
	require.Same(t, engine, c.workflowEngine())

	// Explicit agent MCP restrictions still apply to a refreshed palette.
	cfg.Config().Agents[config.AgentCoder] = config.Agent{AllowedTools: []string{"bash", "workflow"}, AllowedMCP: map[string][]string{}}
	require.NoError(t, c.RefreshTools(ctx))
	require.Len(t, c.ToolCatalog(), 2)
	for _, original := range native {
		require.Same(t, original, find(original.Info().Name))
	}
	require.Empty(t, model.calls, "refreshing tools never generates a model request")
}

type canonicalContextModel struct {
	finishStreamModel
	calls []fantasy.Call
	first func() error
}

func (m *canonicalContextModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls = append(m.calls, call)
	if len(m.calls) == 1 {
		if err := m.first(); err != nil {
			return nil, err
		}
		return func(yield func(fantasy.StreamPart) bool) {
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "context-call", ToolCallName: "context_probe", ToolCallInput: "{}"}) {
				return
			}
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
		}, nil
	}
	return m.finishStreamModel.Stream(ctx, call)
}

func canonicalDynamicContextRefresh(t *testing.T) {
	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	env := testEnv(t)
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = env.workingDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	runGit("init", "--quiet")
	runGit("symbolic-ref", "HEAD", "refs/heads/canonical-before")
	cache := filepath.Join(env.workingDir, ".megafloyd", ".supercache")
	require.NoError(t, os.MkdirAll(filepath.Dir(cache), 0o700))
	require.NoError(t, os.WriteFile(cache, []byte("CACHE_BEFORE"), 0o600))
	first, err := env.sessions.Create(ctx, "first")
	require.NoError(t, err)
	other, err := env.sessions.Create(ctx, "other")
	require.NoError(t, err)
	for _, id := range []string{first.ID, other.ID} {
		_, err := env.messages.Create(ctx, id, message.CreateMessageParams{
			Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Existing session"}},
		})
		require.NoError(t, err)
	}
	tool := fantasy.NewAgentTool("context_probe", "Continue a tool loop", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("context probe complete"), nil
	})
	model := &canonicalContextModel{finishStreamModel: finishStreamModel{text: "done"}}
	a := testSessionAgent(env, model, model, "canonical static prefix", tool).(*sessionAgent)
	captures := 0
	a.dynamicContextFunc = func(ctx context.Context) string {
		captures++
		return prompt.DynamicContext(ctx, env.workingDir, config.Config{})
	}
	a.SetDynamicContext("STALE_SHARED_CONTEXT")
	model.first = func() error {
		runGit("symbolic-ref", "HEAD", "refs/heads/canonical-after")
		require.NoError(t, os.WriteFile(cache, []byte("CACHE_AFTER"), 0o600))
		a.SetDynamicContext("OTHER_RUN_MUST_NOT_OVERWRITE")
		_, err := a.Run(ctx, SessionAgentCall{SessionID: other.ID, Prompt: "other overlapping turn"})
		return err
	}

	// A queued call must capture state when it executes, not when accepted.
	a.activeRequests.Set(first.ID, &activeCancel{cancel: func() {}})
	_, err = a.Run(ctx, SessionAgentCall{SessionID: first.ID, RunID: "queued", Prompt: "queued next turn"})
	require.NoError(t, err)
	require.Zero(t, captures)
	require.Empty(t, model.calls)
	a.activeRequests.Del(first.ID)
	_, err = a.Run(ctx, SessionAgentCall{SessionID: first.ID, Prompt: "first actual turn"})
	require.NoError(t, err)
	require.Equal(t, 3, captures, "one snapshot per actual run, including queued runs")
	require.Len(t, model.calls, 4, "first tool loop has two steps; other and queued runs each have one")
	for i, call := range model.calls {
		text := intelligenceCallText(call)
		require.Contains(t, text, "canonical static prefix")
		require.Contains(t, text, "Today's date: "+time.Now().Format("1/2/2006"))
		require.NotContains(t, text, "STALE_SHARED_CONTEXT")
		require.NotContains(t, text, "OTHER_RUN_MUST_NOT_OVERWRITE")
		require.Equal(t, 1, strings.Count(text, "<env>"))
		if i == 0 || i == 2 {
			require.Contains(t, text, "canonical-before")
			require.Contains(t, text, "CACHE_BEFORE")
			require.NotContains(t, text, "CACHE_AFTER", "another run cannot replace an existing tool loop's snapshot")
		} else {
			require.Contains(t, text, "canonical-after")
			require.Contains(t, text, "CACHE_AFTER")
			require.NotContains(t, text, "CACHE_BEFORE")
		}
	}
}
