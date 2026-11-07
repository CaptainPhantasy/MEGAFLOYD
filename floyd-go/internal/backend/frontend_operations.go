package backend

import (
	"context"

	"github.com/legacy-ai/floyd/internal/frontendops"
	"github.com/legacy-ai/floyd/internal/proto"
)

func (b *Backend) FrontendOperation(ctx context.Context, wid string, in proto.FrontendRequest) (proto.FrontendResult, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return proto.FrontendResult{}, err
	}
	return frontendops.Run(ctx, frontendops.Environment{Config: ws.Cfg, Sessions: ws.Sessions, Messages: ws.Messages, Hooks: ws.Hooks}, in)
}
