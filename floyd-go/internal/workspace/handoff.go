package workspace

import (
	"context"
	"errors"

	"github.com/legacy-ai/floyd/internal/megafloydhandoff"
)

// Handoff uses the same native server that owns this workspace.
func Handoff(ctx context.Context, w Workspace, sessionID, target string) (megafloydhandoff.Descriptor, error) {
	client, ok := w.(*ClientWorkspace)
	if !ok {
		return megafloydhandoff.Descriptor{}, errors.New("handoff requires the shared MegaFloyd server runtime")
	}
	return client.client.Handoff(ctx, client.workspaceID(), sessionID, target)
}
