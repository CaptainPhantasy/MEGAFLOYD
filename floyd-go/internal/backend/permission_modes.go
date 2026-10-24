package backend

import (
	"fmt"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/permission"
)

func (b *Backend) GetPermissionsMode(workspaceID string) (string, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	if svc, ok := ws.Permissions.(permission.ModeService); ok {
		return svc.Mode(), nil
	}
	if ws.Permissions.SkipRequests() {
		return permission.ModeAuto, nil
	}
	return permission.ModeManual, nil
}

func (b *Backend) SetPermissionsMode(workspaceID, mode string) error {
	if mode != permission.ModeManual && mode != permission.ModeYolo && mode != permission.ModeAuto {
		return fmt.Errorf("unknown permission mode %q", mode)
	}
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	svc, ok := ws.Permissions.(permission.ModeService)
	if !ok {
		return fmt.Errorf("permission policy service unavailable")
	}
	if err := ws.Cfg.SetConfigField(config.ScopeWorkspace, "permissions.mode", mode); err != nil {
		return err
	}
	if err := svc.SetMode(mode); err != nil {
		return err
	}
	publishConfigChanged(ws)
	return nil
}
