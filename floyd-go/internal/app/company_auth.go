package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/legacy-ai/floyd/internal/version"
)

// companyTokenSource is the part of megafloydauth.Manager the provider
// wiring needs. Tests substitute a fake.
type companyTokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// companyTokenSourceFactory builds the default token source from the
// environment. Tests replace it.
var companyTokenSourceFactory = func() (companyTokenSource, error) {
	cfg, err := megafloydauth.WithStoredEndpoints(megafloydauth.FromEnv(nil))
	if err != nil {
		return nil, err
	}
	if cfg.Version == "" {
		cfg.Version = version.Version
	}
	return megafloydauth.NewManager(cfg)
}

// companyAuthExecutable locates the running binary for the refresh command.
// Tests replace it.
var companyAuthExecutable = os.Executable

// applyCompanyAuthCredentials replaces selected company-auth api_key
// placeholders with a bearer token from the stored device login. It runs
// before the coordinator builds fantasy providers, so the placeholder never
// reaches a provider. The refresh template installed here lets the
// coordinator call back into `auth token` (and therefore Manager.Token's
// lazy refresh) after a 401.
//
// Unselected account providers never require a login during startup. A newly
// selected provider is resolved by the coordinator when it is built. This keeps
// an optional account logout from preventing another provider from starting.
func applyCompanyAuthCredentials(ctx context.Context, store *config.ConfigStore) error {
	if store == nil {
		return nil
	}
	cfg := store.Config()
	if cfg == nil {
		return nil
	}
	selected := make(map[string]bool)
	for _, model := range cfg.Models {
		selected[model.Provider] = true
	}
	var providerIDs []string
	for _, id := range config.CompanyAuthProviderIDs(cfg) {
		if selected[id] {
			providerIDs = append(providerIDs, id)
		}
	}

	if len(providerIDs) == 0 {
		return nil
	}

	source, err := companyTokenSourceFactory()
	if err != nil {
		return fmt.Errorf("company auth: %w", err)
	}
	accessToken, err := source.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("company auth: %w; run `megafloyd auth login`", err)
	}
	if accessToken == "" {
		return fmt.Errorf("company auth: stored login has no access token; run `megafloyd auth login`")
	}

	executable, err := companyAuthExecutable()
	if err != nil {
		return fmt.Errorf("company auth: locate executable: %w", err)
	}
	refreshCommand := config.CompanyAuthRefreshCommand(executable)

	applied := make([]string, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		if store.SetProviderCompanyAuthCredential(providerID, accessToken, refreshCommand) {
			applied = append(applied, providerID)
		}
	}
	if len(applied) == 0 {
		return fmt.Errorf("company auth: no provider accepted the credential")
	}
	slog.Debug("Applied company-auth credential to providers", "providers", applied)
	return nil
}
