package workspace

import (
	"context"

	"github.com/legacy-ai/floyd/internal/config"
)

type RuntimeOptionsWorkspace interface {
	ApplyRuntimeOptions(context.Context, config.RuntimeOptions) error
}

func (w *AppWorkspace) ApplyRuntimeOptions(_ context.Context, options config.RuntimeOptions) error {
	return w.app.ApplyRuntimeOptions(options)
}

func (w *ClientWorkspace) ApplyRuntimeOptions(ctx context.Context, options config.RuntimeOptions) error {
	if err := w.client.ApplyRuntimeOptions(ctx, w.workspaceID(), options); err != nil {
		return err
	}
	updated, err := w.client.GetWorkspace(ctx, w.workspaceID())
	if err != nil {
		return err
	}
	if updated.Config != nil {
		updated.Config.SetupAgents()
		updated.Config.NormalizeOptions()
	}
	w.mu.Lock()
	w.ws = *updated
	w.mu.Unlock()
	return nil
}
