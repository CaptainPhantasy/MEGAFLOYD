package backend

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/session"
)

func (b *Backend) GetSessionRunSettings(ctx context.Context, workspaceID, sessionID string) (agent.SessionRunSettings, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return agent.SessionRunSettings{}, err
	}
	stored, err := ws.Sessions.Get(ctx, sessionID)
	if err != nil {
		return agent.SessionRunSettings{}, err
	}
	return agent.ReadSessionRunSettings(stored.Metadata)
}

func (b *Backend) UpdateSessionRunSettings(ctx context.Context, workspaceID, sessionID string, patch map[string]json.RawMessage) (agent.SessionRunSettings, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return agent.SessionRunSettings{}, err
	}
	mutator, ok := ws.Sessions.(session.MetadataMutator)
	if !ok {
		return agent.SessionRunSettings{}, fmt.Errorf("session service cannot persist agent settings")
	}
	var result agent.SessionRunSettings
	_, err = mutator.MutateMetadata(ctx, sessionID, func(metadata map[string]any) error {
		var parseErr error
		result, parseErr = agent.PatchSessionRunSettings(metadata, patch)
		if parseErr != nil {
			return parseErr
		}
		if validator, ok := ws.AgentCoordinator.(interface {
			ValidateSessionRunSettings(agent.SessionRunSettings) error
		}); ok {
			return validator.ValidateSessionRunSettings(result)
		}
		return nil
	})
	return result, err
}
