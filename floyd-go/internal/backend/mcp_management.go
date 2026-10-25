package backend

import (
	"context"
	"fmt"

	mcptools "github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

func (b *Backend) TestMCPConnection(ctx context.Context, workspaceID, name string, draft *config.MCPConfig) (mcptools.ConnectionTestResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return mcptools.ConnectionTestResult{}, err
	}
	var server config.MCPConfig
	if draft != nil {
		server = *draft
	} else {
		var exists bool
		server, exists = ws.Cfg.Config().MCP[name]
		if !exists {
			return mcptools.ConnectionTestResult{}, fmt.Errorf("mcp %q is not configured", name)
		}
	}
	return mcptools.TestConnection(ctx, ws.Cfg, name, server), nil
}

func (b *Backend) ResetMCPAuthentication(workspaceID, name string) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	if err = mcptools.ResetAuthentication(ws.Cfg, name); err != nil {
		return err
	}
	ws.SendEvent(pubsub.Event[proto.ConfigChanged]{Type: pubsub.UpdatedEvent, Payload: proto.ConfigChanged{WorkspaceID: ws.ID}})
	return nil
}
