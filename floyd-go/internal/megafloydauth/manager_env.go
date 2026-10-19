package megafloydauth

import (
	"errors"
	"fmt"
)

// NewManager builds the default manager for cfg: the HTTP device flow and
// the file store under the resolved credentials directory. It refuses to
// build a manager that cannot reach a host or a credential slot, so a
// misconfigured environment fails before a provider request is attempted.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.OAuthHost == "" {
		return nil, fmt.Errorf(
			"%w: set MEGAFLOYD_AUTH_HOST or FLOYD_CODE_OAUTH_HOST",
			ErrNotConfigured,
		)
	}
	dir := cfg.CredentialsPath()
	if dir == "" {
		return nil, fmt.Errorf("%w: no credentials directory", ErrNotConfigured)
	}
	name, err := StorageName(CredentialKey(cfg.OAuthHost, cfg.BaseURL))
	if err != nil {
		return nil, err
	}
	return &Manager{
		Flow:  NewClient(cfg),
		Store: FileStore{Dir: dir},
		Name:  name,
		Now:   cfg.Now,
		Sleep: cfg.Sleep,
	}, nil
}

// CredentialPath returns the file path backing the manager's token slot. It
// is available only for the file store used by NewManager.
func (m *Manager) CredentialPath() (string, error) {
	if m == nil {
		return "", fmt.Errorf("%w: manager is nil", ErrNotConfigured)
	}
	store, ok := m.Store.(FileStore)
	if !ok {
		return "", errors.New("megafloydauth: token store has no file path")
	}
	return store.Path(m.Name)
}
