package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	mcpclient "github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/permission"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type canonicalDeferredPermissions struct {
	permission.Service
	allow    bool
	requests []permission.CreatePermissionRequest
}

func (p *canonicalDeferredPermissions) Request(_ context.Context, request permission.CreatePermissionRequest) (bool, error) {
	p.requests = append(p.requests, request)
	return p.allow, nil
}

func TestCanonicalDeferredMCPUsesTargetPermissionsAndSettings(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const serverName = "canonical-deferred"
	const targetName = "mcp_canonical-deferred_probe"
	const hiddenName = "mcp_canonical-deferred_excluded"
	var executions atomic.Int32
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: serverName}, nil)
	for _, name := range []string{"probe", "excluded"} {
		mcpsdk.AddTool(server, &mcpsdk.Tool{Name: name, Description: "Read gateway status"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args struct {
			Query string `json:"query"`
		}) (*mcpsdk.CallToolResult, any, error) {
			executions.Add(1)
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "gateway " + args.Query}}}, nil, nil
		})
	}
	httpServer := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()
	env := testEnv(t)
	sess, err := env.sessions.Create(ctx, "deferred MCP verification")
	require.NoError(t, err)
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, sess.ID)
	sessionless := true
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{
		Options: &config.Options{DataDirectory: t.TempDir()},
		Agents:  map[string]config.Agent{config.AgentCoder: {}},
		MCP:     config.MCPs{serverName: {Type: config.MCPHttp, URL: httpServer.URL, Sessionless: &sessionless, Deferred: true}},
	}, env.workingDir)
	permissions := &canonicalDeferredPermissions{Service: env.permissions, allow: true}
	model := &intelligenceModel{}
	a := testSessionAgent(env, model, model, "canonical deferred prompt").(*sessionAgent)
	c := &coordinator{cfg: cfg, permissions: permissions, currentAgent: a}
	require.NoError(t, mcpclient.InitializeSingle(ctx, serverName, cfg))
	defer func() { require.NoError(t, mcpclient.DisableSingle(cfg, serverName)) }()
	a.SetTools(c.buildMCPTools(config.Agent{}, false))
	require.Len(t, c.ToolCatalog(), 2)
	for _, info := range c.ToolCatalog() {
		require.Contains(t, []string{deferredMCPSearchName, deferredMCPCallName}, info.Name, "deferred target schemas stay out of the initial palette")
	}
	allowed := []string{targetName}
	settings := SessionRunSettings{Tools: &allowed}
	find := func(name string) fantasy.AgentTool {
		for _, tool := range settings.filterTools(a.tools.Copy()) {
			if tool.Info().Name == name {
				return tool
			}
		}
		t.Fatalf("tool %s not found", name)
		return nil
	}
	result, err := find(deferredMCPSearchName).Run(ctx, fantasy.ToolCall{ID: "discover", Name: deferredMCPSearchName, Input: `{"query":"gateway"}`})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Contains(t, result.Content, targetName)
	require.NotContains(t, result.Content, hiddenName)
	require.Contains(t, result.Content, `"query":{"type":"string"}`)
	require.Empty(t, permissions.requests, "schema discovery does not execute the target")
	call := func(name string) fantasy.ToolResponse {
		input, err := json.Marshal(map[string]any{"name": name, "arguments": map[string]any{"query": "ready"}})
		require.NoError(t, err)
		result, err := find(deferredMCPCallName).Run(ctx, fantasy.ToolCall{ID: "deferred-call", Name: deferredMCPCallName, Input: string(input)})
		require.NoError(t, err)
		return result
	}
	require.True(t, call(hiddenName).IsError, "generic execution cannot bypass the captured target allowlist")
	require.Empty(t, permissions.requests)
	result = call(targetName)
	require.False(t, result.IsError)
	require.Equal(t, "gateway ready", result.Content)
	require.EqualValues(t, 1, executions.Load())
	require.Len(t, permissions.requests, 1)
	require.Equal(t, targetName, permissions.requests[0].ToolName)
	require.Equal(t, "deferred-call", permissions.requests[0].ToolCallID)
	require.JSONEq(t, `{"query":"ready"}`, permissions.requests[0].Params.(string))
	permissions.allow = false
	require.True(t, call(targetName).IsError)
	require.EqualValues(t, 1, executions.Load(), "denied target permissions prevent remote execution")
	permissions.allow = true
	cfg.Config().Hooks = map[string][]config.HookConfig{hooks.EventPreToolUse: {{Command: `echo '{"decision":"deny","reason":"deferred-target-hook"}'`, Matcher: "^" + targetName + "$", Timeout: 2}}}
	require.NoError(t, c.RefreshTools(ctx))
	require.Len(t, c.ToolCatalog(), 2, "refresh replaces deferred meta-tools without duplicates")
	result = call(targetName)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, "deferred-target-hook")
	require.Len(t, permissions.requests, 2, "target hook runs before target permission")
	require.EqualValues(t, 1, executions.Load())
	none := []string{}
	require.Empty(t, (SessionRunSettings{MCPServers: &none}).filterTools(a.tools.Copy()))
	require.Empty(t, (SessionRunSettings{Tools: &none}).filterTools(a.tools.Copy()))
	require.Empty(t, (SessionRunSettings{Tools: &allowed, DisabledTools: allowed}).filterTools(a.tools.Copy()))
	require.Empty(t, model.calls, "discovery and target tool checks use no model")
}
