package megafloydauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/legacy-ai/floyd/internal/lock"
)

// WithStoredEndpoints resolves only the native account metadata in MEGAFLOYD's
// own config. It never imports tokens or configuration from another harness.
func WithStoredEndpoints(cfg Config) (Config, error) {
	if cfg.HomeDir == "" {
		return cfg, nil
	}
	var base, host, clientID, tokenPath string
	for _, dir := range []string{"config", "data"} {
		path := filepath.Join(cfg.HomeDir, dir, "floyd.json")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return cfg, fmt.Errorf("read company account config: %w", err)
		}
		var file struct {
			Options struct {
				CompanyURL string `json:"company_platform_url"`
				Accounts   map[string]struct {
					BaseURL   string `json:"base_url"`
					OAuthHost string `json:"oauth_host"`
					ClientID  string `json:"client_id"`
					TokenPath string `json:"token_path"`
				} `json:"managed_accounts"`
			} `json:"options"`
		}
		if err = json.Unmarshal(data, &file); err != nil {
			return cfg, fmt.Errorf("decode company account config: %w", err)
		}
		a := file.Options.Accounts["managed:floyd-code"]
		base = firstNonEmpty(a.BaseURL, file.Options.CompanyURL, base)
		host = firstNonEmpty(a.OAuthHost, file.Options.CompanyURL, host)
		clientID = firstNonEmpty(a.ClientID, clientID)
		tokenPath = firstNonEmpty(a.TokenPath, tokenPath)
	}
	if cfg.OAuthHost == "" {
		cfg.OAuthHost = host
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = base
	}
	if clientID != "" && cfg.ClientID == DefaultClientID {
		cfg.ClientID = clientID
	}
	if cfg.TokenPath == "" {
		cfg.TokenPath = tokenPath
	}
	return cfg, nil
}

func (m *Manager) lockSlot(ctx context.Context) (func(), error) {
	if _, ok := m.Store.(FileStore); !ok {
		return func() {}, nil
	}
	path, err := m.CredentialPath()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return lock.File(bounded, path+".refresh.lock")
}

func (m *Manager) WithCredentialLock(ctx context.Context, fn func() error) error {
	if m == nil || m.Store == nil || m.Name == "" {
		return fmt.Errorf("%w: manager has no token store", ErrNotConfigured)
	}
	release, err := m.lockSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}
func (m *Manager) SaveToken(ctx context.Context, token Token) error {
	return m.WithCredentialLock(ctx, func() error { return m.Store.Save(m.Name, token) })
}
