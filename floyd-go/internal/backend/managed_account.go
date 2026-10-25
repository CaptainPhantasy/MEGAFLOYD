package backend

import (
	"context"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/oauth"
)

func (b *Backend) GetManagedAccount(wid, provider string) (config.ManagedAccount, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return config.ManagedAccount{}, err
	}
	return ws.Cfg.ManagedAccount(provider)
}
func (b *Backend) SaveManagedAccount(wid string, req config.ManagedAccountLogin) error {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return err
	}
	if err = ws.Cfg.SaveManagedAccount(req); err != nil {
		return err
	}
	publishConfigChanged(ws)
	return nil
}
func (b *Backend) LogoutManagedAccount(wid, provider string) error {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return err
	}
	if err = ws.Cfg.LogoutManagedAccount(provider); err != nil {
		return err
	}
	publishConfigChanged(ws)
	return nil
}
func (b *Backend) GetManagedAccountToken(wid, provider string) (*oauth.Token, error) {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return nil, err
	}
	return ws.Cfg.ManagedAccountToken(provider)
}
func (b *Backend) RefreshManagedAccount(ctx context.Context, wid, provider string) error {
	ws, err := b.GetWorkspace(wid)
	if err != nil {
		return err
	}
	if err = ws.Cfg.RefreshManagedAccount(ctx, provider); err != nil {
		return err
	}
	publishConfigChanged(ws)
	return nil
}
