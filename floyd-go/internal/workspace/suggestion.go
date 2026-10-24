package workspace

import "context"

func (w *AppWorkspace) SuggestPrompt(ctx context.Context, sessionID, prefix string) (string, error) {
	if w.app.AgentCoordinator == nil {
		return "", ErrAgentNotInitialized
	}
	return w.app.AgentCoordinator.SuggestPrompt(ctx, sessionID, prefix)
}

func (w *ClientWorkspace) SuggestPrompt(ctx context.Context, sessionID, prefix string) (string, error) {
	return w.client.SuggestPrompt(ctx, w.workspaceID(), sessionID, prefix)
}
