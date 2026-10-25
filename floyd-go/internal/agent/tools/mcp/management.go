package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type connectionProbeKey struct{}

type ConnectionTestResult struct {
	Success bool    `json:"success"`
	Output  string  `json:"output"`
	Tools   []*Tool `json:"tools,omitempty"`
}

func TestConnection(ctx context.Context, cfg *config.ConfigStore, name string, server config.MCPConfig) ConnectionTestResult {
	if name == "" {
		name = "connection-test"
	}
	ctx, cancel := context.WithTimeout(context.WithValue(ctx, connectionProbeKey{}, true), mcpTimeout(server))
	defer cancel()
	transport, handler, err := createTransport(ctx, cfg, name, server, cfg.Resolver())
	if err != nil {
		return ConnectionTestResult{Output: err.Error()}
	}
	if handler != nil {
		defer handler.Close()
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "MegaFloyd-connection-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return ConnectionTestResult{Output: maybeTimeoutErr(err, mcpTimeout(server)).Error()}
	}
	defer session.Close()
	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return ConnectionTestResult{Output: err.Error()}
	}
	tools := filterTools(server, result.Tools)
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return ConnectionTestResult{Success: true, Output: fmt.Sprintf("Connected successfully. %d tools: %s", len(tools), strings.Join(names, ", ")), Tools: tools}
}

func ResetAuthentication(cfg *config.ConfigStore, name string) error {
	server, exists := cfg.Config().MCP[name]
	if !exists {
		return fmt.Errorf("mcp %q is not configured", name)
	}
	if !server.OAuth {
		return fmt.Errorf("mcp %q does not use OAuth", name)
	}
	key := registryName(cfg, name)
	lock := suppressLock(key)
	if !lock.TryLock() {
		return fmt.Errorf("cancel the active authentication for mcp %q before resetting it", name)
	}
	defer lock.Unlock()
	mu := renewLock(key)
	mu.Lock()
	defer mu.Unlock()
	if err := cfg.ClearMCPOAuthToken(name); err != nil {
		return err
	}
	teardown(key)
	if server.Disabled {
		updateState(key, StateDisabled, nil, nil, Counts{})
	} else {
		updateState(key, StateNeedsAuth, nil, nil, Counts{})
	}
	return nil
}
