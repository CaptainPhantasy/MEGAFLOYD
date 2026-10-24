package backend

import "context"

// SuggestPrompt uses the workspace's configured agent and small model.
func (b *Backend) SuggestPrompt(ctx context.Context, workspaceID, sessionID, prefix string) (string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	if ws.AgentCoordinator == nil {
		return "", ErrAgentNotInitialized
	}
	return ws.AgentCoordinator.SuggestPrompt(ctx, sessionID, prefix)
}
