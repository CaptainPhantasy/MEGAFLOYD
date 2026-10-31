package workspace

import (
	"context"

	"github.com/legacy-ai/floyd/internal/frontendops"
	"github.com/legacy-ai/floyd/internal/proto"
)

type FrontendOperations interface {
	FrontendOperation(context.Context, proto.FrontendRequest) (proto.FrontendResult, error)
}

func (w *AppWorkspace) FrontendOperation(ctx context.Context, in proto.FrontendRequest) (proto.FrontendResult, error) {
	return frontendops.Run(ctx, frontendops.Environment{Config: w.store, Sessions: w.app.Sessions, Messages: w.app.Messages, Hooks: w.app.Hooks}, in)
}

func (w *ClientWorkspace) FrontendOperation(ctx context.Context, in proto.FrontendRequest) (proto.FrontendResult, error) {
	return w.client.FrontendOperation(ctx, w.workspaceID(), in)
}
