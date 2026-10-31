package backend

import (
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
)

func (b *Backend) ApplyRuntimeOptions(id string, options config.RuntimeOptions) error {
	ws, err := b.GetWorkspace(id)
	if err != nil {
		return err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	apply := func() error {
		if err := ws.Cfg.ApplyRuntimeOptions(options); err != nil {
			return err
		}
		if options.PermissionMode != nil {
			if err := ws.Permissions.(permission.ModeService).SetMode(*options.PermissionMode); err != nil {
				return err
			}
		}
		if options.AllowedTools != nil {
			ws.Permissions.(permission.RuntimeAllowlist).SetAllowedTools(options.AllowedTools)
		}
		ws.Permissions.SetAdditionalDirs(ws.Cfg.Config().Options.AdditionalDirs)
		return nil
	}
	if c, ok := ws.AgentCoordinator.(interface{ RuntimeWithIdle(func() error) error }); ok {
		return c.RuntimeWithIdle(apply)
	}
	if ws.AgentCoordinator != nil && ws.AgentCoordinator.IsBusy() {
		return agent.ErrSessionBusy
	}
	return apply()
}
