package app

import (
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
)

func newConfiguredPermissions(dir string, skip bool, allowed []string, cfg *config.Config) (permission.Service, error) {
	svc := permission.NewPermissionService(dir, skip, allowed)
	if !skip && cfg.Permissions != nil && cfg.Permissions.Mode != "" {
		if err := svc.(permission.ModeService).SetMode(cfg.Permissions.Mode); err != nil {
			return nil, err
		}
	}
	return svc, nil
}
