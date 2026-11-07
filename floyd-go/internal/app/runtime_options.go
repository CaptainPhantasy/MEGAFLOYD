package app

import (
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
)

func (app *App) ApplyRuntimeOptions(options config.RuntimeOptions) error {
	apply := func() error {
		if err := app.config.ApplyRuntimeOptions(options); err != nil {
			return err
		}
		if options.PermissionMode != nil {
			if err := app.Permissions.(permission.ModeService).SetMode(*options.PermissionMode); err != nil {
				return err
			}
		}
		if options.AllowedTools != nil {
			app.Permissions.(permission.RuntimeAllowlist).SetAllowedTools(options.AllowedTools)
		}
		app.Permissions.SetAdditionalDirs(app.config.Config().Options.AdditionalDirs)
		return nil
	}
	if c, ok := app.AgentCoordinator.(interface{ RuntimeWithIdle(func() error) error }); ok {
		return c.RuntimeWithIdle(apply)
	}
	if app.AgentCoordinator != nil && app.AgentCoordinator.IsBusy() {
		return agent.ErrSessionBusy
	}
	return apply()
}
