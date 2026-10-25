package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/legacy-ai/floyd/internal/oauth"
	"github.com/legacy-ai/floyd/internal/version"
)

const ManagedAccountProvider = "managed:floyd-code"

type ManagedAccount struct {
	Provider      string `json:"provider"`
	BaseURL       string `json:"base_url"`
	OAuthHost     string `json:"oauth_host"`
	Region        string `json:"region"`
	ClientID      string `json:"client_id,omitempty"`
	TokenPath     string `json:"token_path,omitempty"`
	Authenticated bool   `json:"authenticated"`
	Status        string `json:"status"`
}
type ManagedAccountLogin struct {
	Provider  string       `json:"provider"`
	BaseURL   string       `json:"base_url"`
	OAuthHost string       `json:"oauth_host"`
	Region    string       `json:"region,omitempty"`
	Token     *oauth.Token `json:"token"`
}

var accountProviderID = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)

func normalizeAccountProvider(id string) (string, error) {
	if id == "" {
		id = ManagedAccountProvider
	}
	if !accountProviderID.MatchString(id) || !strings.HasPrefix(id, "managed:") {
		return "", fmt.Errorf("invalid managed account provider")
	}
	return id, nil
}
func accountURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("account endpoint must be an HTTP or HTTPS URL")
	}
	return u, nil
}
func (s *ConfigStore) accountMetadata(provider string) (ManagedAccount, error) {
	id, err := normalizeAccountProvider(provider)
	if err != nil {
		return ManagedAccount{}, err
	}
	cfg := s.Config()
	a := ManagedAccount{Provider: id, Region: "global", Status: "unauthenticated"}
	if cfg.Options != nil {
		if stored, ok := cfg.Options.ManagedAccounts[id]; ok {
			a = stored
			a.Provider = id
			a.Authenticated = false
			a.Status = "unauthenticated"
		}
		if a.BaseURL == "" {
			a.BaseURL = cfg.Options.CompanyPlatformURL
		}
		if a.OAuthHost == "" {
			a.OAuthHost = cfg.Options.CompanyPlatformURL
		}
	}
	env := megafloydauth.FromEnv(nil)
	if a.BaseURL == "" {
		a.BaseURL = os.Getenv("MEGAFLOYD_COMPANY_PLATFORM_URL")
	}
	if a.OAuthHost == "" {
		a.OAuthHost = os.Getenv("MEGAFLOYD_COMPANY_PLATFORM_URL")
	}
	if a.BaseURL == "" {
		a.BaseURL = env.BaseURL
	}
	if a.OAuthHost == "" {
		a.OAuthHost = env.OAuthHost
	}
	if a.Region == "" {
		a.Region = "global"
	}
	return a, nil
}
func accountManager(a ManagedAccount) (*megafloydauth.Manager, error) {
	cfg := megafloydauth.FromEnv(nil)
	cfg.OAuthHost = a.OAuthHost
	cfg.BaseURL = a.BaseURL
	if a.ClientID != "" {
		cfg.ClientID = a.ClientID
	}
	if a.TokenPath != "" {
		cfg.TokenPath = a.TokenPath
	}
	cfg.Version = version.Version
	if cfg.Version == "" {
		cfg.Version = "devel"
	}
	return megafloydauth.NewManager(cfg)
}
func (s *ConfigStore) ManagedAccount(provider string) (ManagedAccount, error) {
	a, err := s.accountMetadata(provider)
	if err != nil {
		return a, err
	}
	if a.OAuthHost == "" {
		return a, nil
	}
	m, err := accountManager(a)
	if err != nil {
		return a, err
	}
	status, err := m.Status()
	if err != nil {
		return a, err
	}
	switch {
	case status.Revoked:
		a.Status = "revoked"
	case status.LoggedIn:
		if !status.ExpiresAt.IsZero() && !status.ExpiresAt.After(time.Now()) {
			a.Status = "expired"
		} else {
			a.Authenticated = true
			a.Status = "authenticated"
		}
	}
	return a, nil
}
func (s *ConfigStore) ManagedAccountToken(provider string) (*oauth.Token, error) {
	a, err := s.accountMetadata(provider)
	if err != nil {
		return nil, err
	}
	if a.OAuthHost == "" {
		return nil, nil
	}
	m, err := accountManager(a)
	if err != nil {
		return nil, err
	}
	token, exists, err := m.Store.Load(m.Name)
	if err != nil {
		return nil, err
	}
	if !exists || !token.Valid() {
		return nil, nil
	}
	return token.OAuthToken(), nil
}
func (s *ConfigStore) RefreshManagedAccount(ctx context.Context, provider string) error {
	a, err := s.accountMetadata(provider)
	if err != nil {
		return err
	}
	m, err := accountManager(a)
	if err != nil {
		return err
	}
	_, err = m.Token(ctx)
	if err == nil {
		s.SignalAuthComplete(a.Provider)
	}
	return err
}
func (s *ConfigStore) SaveManagedAccount(req ManagedAccountLogin) error {
	id, err := normalizeAccountProvider(req.Provider)
	if err != nil {
		return err
	}
	if _, err = accountURL(req.BaseURL); err != nil {
		return err
	}
	host, err := accountURL(req.OAuthHost)
	if err != nil {
		return err
	}
	if req.Region == "" {
		req.Region = "global"
	}
	if req.Region != "global" && req.Region != "mainland-cn" {
		return fmt.Errorf("invalid account region")
	}
	if req.Token == nil || req.Token.AccessToken == "" {
		return fmt.Errorf("account access token is required")
	}
	token := *req.Token
	if token.ExpiresAt <= 0 {
		token.SetExpiresAt()
	}
	if token.ExpiresAt <= 0 {
		return fmt.Errorf("account token expiry is required")
	}
	a := ManagedAccount{Provider: id, BaseURL: strings.TrimRight(req.BaseURL, "/"), OAuthHost: strings.TrimRight(req.OAuthHost, "/"), Region: req.Region}
	if token.Client != nil {
		u, e := accountURL(token.Client.TokenURL)
		if e != nil || u.Scheme != host.Scheme || !strings.EqualFold(u.Host, host.Host) {
			return fmt.Errorf("account token endpoint must belong to the configured OAuth host")
		}
		a.ClientID = token.Client.ClientID
		a.TokenPath = u.Path
	}
	m, err := accountManager(a)
	if err != nil {
		return err
	}
	nativeToken := megafloydauth.TokenFromOAuth(&token)
	nativeToken.Scope = "floyd-code"
	nativeToken.TokenType = "Bearer"
	err = m.WithCredentialLock(context.Background(), func() error {
		previous, hadPrevious, err := m.Store.Load(m.Name)
		if err != nil {
			return err
		}
		if err = m.Store.Save(m.Name, nativeToken); err != nil {
			return err
		}
		err = s.updateManagedAccount(func(cfg *Config) map[string]any {
			p, _ := cfg.Providers.Get(id)
			p.ID = id
			if p.Name == "" {
				p.Name = "Company account"
			}
			if p.Type == "" {
				p.Type = catwalk.TypeOpenAICompat
			}
			p.BaseURL = a.BaseURL
			p.APIKey = CompanyAuthAPIKeyScheme + "default"
			p.APIKeyTemplate = ""
			p.OAuthToken = nil
			p.Disable = false
			no := false
			p.AutoDiscoverModels = &no
			cfg.Providers.Set(id, p)
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			accounts := maps.Clone(cfg.Options.ManagedAccounts)
			if accounts == nil {
				accounts = map[string]ManagedAccount{}
			}
			accounts[id] = a
			cfg.Options.ManagedAccounts = accounts
			return map[string]any{"providers." + id: p, "options.managed_accounts": accounts}
		})
		if err != nil {
			var rollback error
			if hadPrevious {
				rollback = m.Store.Save(m.Name, previous)
			} else {
				rollback = m.Store.Remove(m.Name)
			}
			return errors.Join(err, rollback)
		}
		return nil
	})
	if err == nil {
		s.SignalAuthComplete(id)
	}
	return err
}
func (s *ConfigStore) LogoutManagedAccount(provider string) error {
	a, err := s.accountMetadata(provider)
	if err != nil {
		return err
	}
	update := func() error {
		return s.updateManagedAccount(func(cfg *Config) map[string]any {
			p, ok := cfg.Providers.Get(a.Provider)
			if !ok {
				return nil
			}
			p.APIKey = ""
			p.APIKeyTemplate = ""
			p.OAuthToken = nil
			p.Disable = true
			cfg.Providers.Set(a.Provider, p)
			return map[string]any{"providers." + a.Provider: p}
		})
	}
	if a.OAuthHost == "" {
		return update()
	}
	m, err := accountManager(a)
	if err != nil {
		return err
	}
	return m.WithCredentialLock(context.Background(), func() error {
		previous, existed, err := m.Store.Load(m.Name)
		if err != nil {
			return err
		}
		if err = m.Store.Remove(m.Name); err != nil {
			return err
		}
		if err = update(); err != nil {
			if existed {
				return errors.Join(err, m.Store.Save(m.Name, previous))
			}
			return err
		}
		return nil
	})
}
func (s *ConfigStore) updateManagedAccount(mutate func(*Config) map[string]any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	next := s.Config().cloneForWrite()
	providers := csync.NewMap[string, ProviderConfig]()
	for id, p := range next.Providers.Copy() {
		providers.Set(id, p)
	}
	next.Providers = providers
	fields := mutate(next)
	if len(fields) > 0 {
		if err := s.writeConfigFields(ScopeGlobal, fields); err != nil {
			return err
		}
	}
	s.setConfig(next)
	if path, err := s.configPath(ScopeGlobal); err == nil {
		s.captureStalenessSnapshot(append(slices.Clone(s.loadedPaths), path))
	}
	return nil
}

// ResolveCompanyProviderCredential is called when a selected provider is built.
// The native CLI and web account share the same endpoint-keyed credential slot.
func (s *ConfigStore) ResolveCompanyProviderCredential(ctx context.Context, providerID string) error {
	p, ok := s.Config().Providers.Get(providerID)
	if !ok || !IsCompanyAuthAPIKey(p.APIKey) {
		return nil
	}
	id := ManagedAccountProvider
	if strings.HasPrefix(providerID, "managed:") {
		id = providerID
	}
	a, err := s.accountMetadata(id)
	if err != nil {
		return err
	}
	m, err := accountManager(a)
	if err != nil {
		return err
	}
	access, err := m.AccessToken(ctx)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	s.SetProviderCompanyAuthCredential(providerID, access, CompanyAuthRefreshCommand(executable))
	return nil
}
