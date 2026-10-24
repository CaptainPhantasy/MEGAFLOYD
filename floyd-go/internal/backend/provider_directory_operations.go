package backend

import (
	"context"
	"github.com/legacy-ai/floyd/internal/config"
)

func (b *Backend) RefreshProviderModels(ctx context.Context, workspaceID, provider string, oauthOnly bool) (config.ProviderRefreshResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return config.ProviderRefreshResult{}, err
	}
	result, err := ws.Cfg.RefreshProviderModels(ctx, provider, oauthOnly)
	if err == nil {
		publishConfigChanged(ws)
	}
	return result, err
}

func (b *Backend) AddWorkspaceDirectory(workspaceID, path string, persist bool) (config.AdditionalDirectoryResult, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return config.AdditionalDirectoryResult{}, err
	}
	result, err := ws.Cfg.AddWorkspaceDirectory(path, persist)
	if err != nil {
		return result, err
	}
	publishConfigChanged(ws)
	return result, nil
}
