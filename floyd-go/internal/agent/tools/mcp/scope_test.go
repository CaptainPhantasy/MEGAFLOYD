package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	mcpoauth "github.com/legacy-ai/floyd/internal/oauth/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func scopedFixture(t *testing.T, label string) config.MCPConfig {
	t.Helper()
	server := sdk.NewServer(&sdk.Implementation{Name: label, Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "identity", Description: "Return fixture identity"}, func(context.Context, *sdk.CallToolRequest, struct{}) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: label}}}, nil, nil
	})
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true}))
	t.Cleanup(httpServer.Close)
	sessionless := true
	return config.MCPConfig{Type: config.MCPHttp, URL: httpServer.URL, Sessionless: &sessionless, StartupTimeoutMs: 2000}
}

func TestMCPWorkspaceIsolation(t *testing.T) {
	const name = "same-public-name"
	a := config.NewTestStoreWithWorkingDir(&config.Config{MCP: config.MCPs{name: scopedFixture(t, "workspace-a")}}, t.TempDir())
	b := config.NewTestStoreWithWorkingDir(&config.Config{MCP: config.MCPs{name: scopedFixture(t, "workspace-b")}}, t.TempDir())
	t.Cleanup(func() { _ = Close(context.Background(), a); _ = Close(context.Background(), b) })
	require.NoError(t, InitializeSingle(t.Context(), name, a))
	require.NoError(t, InitializeSingle(t.Context(), name, b))
	for _, entry := range []struct {
		cfg  *config.ConfigStore
		want string
	}{{a, "workspace-a"}, {b, "workspace-b"}} {
		result, err := RunTool(t.Context(), entry.cfg, name, "identity", `{}`)
		require.NoError(t, err)
		require.Equal(t, entry.want, result.Content)
		require.Len(t, GetStates(entry.cfg), 1)
		info, ok := GetState(name, entry.cfg)
		require.True(t, ok)
		require.Equal(t, name, info.Name)
		names := []string{}
		for server := range Tools(entry.cfg) {
			names = append(names, server)
		}
		require.Equal(t, []string{name}, names)
	}
	require.NotContains(t, GetStates(), name)
	var nilStore *config.ConfigStore
	require.NotContains(t, GetStates(nilStore), name)
	events := SubscribeEvents(t.Context(), a)
	other := config.NewTestStoreWithWorkingDir(&config.Config{MCP: config.MCPs{}}, b.WorkingDir())
	Reinitialize(t.Context(), other)
	require.Empty(t, GetStates(b))
	result, err := RunTool(t.Context(), a, name, "identity", `{}`)
	require.NoError(t, err)
	require.Equal(t, "workspace-a", result.Content)
	require.NoError(t, DisableSingle(a, name))
	select {
	case event := <-events:
		require.Equal(t, name, event.Payload.Name)
		require.Equal(t, StateDisabled, event.Payload.State)
	case <-time.After(time.Second):
		t.Fatal("workspace event missing")
	}
	require.NoError(t, InitializeSingle(t.Context(), name, b))
	require.NoError(t, Close(context.Background(), a))
	result, err = RunTool(t.Context(), b, name, "identity", `{}`)
	require.NoError(t, err)
	require.Equal(t, "workspace-b", result.Content)
}

func TestMCPConnectionProbe(t *testing.T) {
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{MCP: config.MCPs{}}, t.TempDir())
	result := TestConnection(t.Context(), cfg, "draft", scopedFixture(t, "probe"))
	require.True(t, result.Success, result.Output)
	require.Len(t, result.Tools, 1)
	require.Empty(t, GetStates(cfg))
	count := 0
	for range Tools(cfg) {
		count++
	}
	require.Zero(t, count)
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); slow.Close() })
	started := time.Now()
	result = TestConnection(t.Context(), cfg, "slow", config.MCPConfig{Type: config.MCPHttp, URL: slow.URL, StartupTimeoutMs: 50})
	require.False(t, result.Success)
	require.NotEmpty(t, result.Output)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestMCPInteractiveSessionOutlivesAuthRequest(t *testing.T) {
	server := scopedFixture(t, "interactive")
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{MCP: config.MCPs{"auth": server}}, t.TempDir())
	ctx, cancel := context.WithCancel(mcpoauth.WithInteractive(t.Context()))
	session, err := createSession(ctx, cfg, registryName(cfg, "auth"), server, cfg.Resolver(), false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(); removeServer(registryName(cfg, "auth")) })
	cancel()
	require.NoError(t, session.Ping(t.Context(), nil))
}
