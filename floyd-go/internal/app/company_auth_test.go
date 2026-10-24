package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/stretchr/testify/require"
)

type fakeCompanyTokenSource struct {
	token string
	err   error
	calls int
}

func (f *fakeCompanyTokenSource) AccessToken(context.Context) (string, error) {
	f.calls++
	return f.token, f.err
}

func useCompanyTokenSource(t *testing.T, src companyTokenSource) {
	t.Helper()
	previous := companyTokenSourceFactory
	companyTokenSourceFactory = func() (companyTokenSource, error) { return src, nil }
	t.Cleanup(func() { companyTokenSourceFactory = previous })
}

func useCompanyExecutable(t *testing.T, path string) {
	t.Helper()
	previous := companyAuthExecutable
	companyAuthExecutable = func() (string, error) { return path, nil }
	t.Cleanup(func() { companyAuthExecutable = previous })
}

func companyAuthConfig() *config.Config {
	cfg := &config.Config{Providers: csync.NewMap[string, config.ProviderConfig](), Models: map[config.SelectedModelType]config.SelectedModel{
		config.SelectedModelTypeLarge: {Provider: "company"}, config.SelectedModelTypeSmall: {Provider: "company-anthropic"},
	}}
	cfg.Providers.Set("company", config.ProviderConfig{
		ID:     "company",
		Type:   catwalk.Type("openai"),
		APIKey: config.CompanyAuthAPIKeyScheme + "default",
	})
	cfg.Providers.Set("company-anthropic", config.ProviderConfig{
		ID:     "company-anthropic",
		Type:   "anthropic",
		APIKey: config.CompanyAuthAPIKeyScheme + "default",
	})
	cfg.Providers.Set("real", config.ProviderConfig{
		ID:     "real",
		Type:   catwalk.Type("openai"),
		APIKey: "sk-real",
	})
	return cfg
}

func TestApplyCompanyAuthCredentialsNoPlaceholders(t *testing.T) {
	source := &fakeCompanyTokenSource{token: "unused"}
	useCompanyTokenSource(t, source)
	useCompanyExecutable(t, "/opt/floyd")
	cfg := &config.Config{Providers: csync.NewMap[string, config.ProviderConfig](), Models: map[config.SelectedModelType]config.SelectedModel{config.SelectedModelTypeLarge: {Provider: "real"}}}
	cfg.Providers.Set("real", config.ProviderConfig{ID: "real", APIKey: "sk-real"})

	require.NoError(t, applyCompanyAuthCredentials(t.Context(), config.NewTestStore(cfg)))
	require.Zero(t, source.calls, "the token source must not run without a placeholder")
}

func TestApplyCompanyAuthCredentialsReplacesPlaceholdersOnly(t *testing.T) {
	source := &fakeCompanyTokenSource{token: "company-token"}
	useCompanyTokenSource(t, source)
	useCompanyExecutable(t, "/opt/floyd")
	store := config.NewTestStore(companyAuthConfig())

	require.NoError(t, applyCompanyAuthCredentials(t.Context(), store))

	got, _ := store.Config().Providers.Get("company")
	require.Equal(t, "company-token", got.APIKey)
	require.Equal(t, "$('/opt/floyd' auth token)", got.APIKeyTemplate)

	got, _ = store.Config().Providers.Get("company-anthropic")
	require.Equal(t, "Bearer company-token", got.APIKey)
	require.Equal(t, "$('/opt/floyd' auth token)", got.APIKeyTemplate)

	got, _ = store.Config().Providers.Get("real")
	require.Equal(t, "sk-real", got.APIKey)
	require.Empty(t, got.APIKeyTemplate)
	require.Equal(t, 1, source.calls, "one token is shared by every provider")
}

func TestApplyCompanyAuthCredentialsFailsWithoutLogin(t *testing.T) {
	source := &fakeCompanyTokenSource{err: megafloydauth.ErrNotLoggedIn}
	useCompanyTokenSource(t, source)
	useCompanyExecutable(t, "/opt/floyd")
	store := config.NewTestStore(companyAuthConfig())

	err := applyCompanyAuthCredentials(t.Context(), store)
	require.ErrorIs(t, err, megafloydauth.ErrNotLoggedIn)

	got, _ := store.Config().Providers.Get("company")
	require.Equal(t, config.CompanyAuthAPIKeyScheme+"default", got.APIKey,
		"a failed lookup must leave the placeholder in place")
}

// TestApplyCompanyAuthRefreshesExpiredTokenOverHTTP drives the real
// megafloydauth client and manager against an httptest server. The seeded
// token is already inside its refresh window, so Manager.Token performs the
// refresh grant; no device authorization or live login is involved.
func TestApplyCompanyAuthRefreshesExpiredTokenOverHTTP(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var (
		mu     sync.Mutex
		grants []url.Values
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != megafloydauth.DefaultTokenPath {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		values := url.Values{}
		for key, value := range r.PostForm {
			values[key] = append([]string(nil), value...)
		}
		grants = append(grants, values)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"access_token": "fresh-access",
			"refresh_token": "fresh-refresh",
			"expires_in": 3600,
			"scope": "floyd-code",
			"token_type": "Bearer"
		}`)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	authCfg := megafloydauth.Config{
		OAuthHost:      server.URL,
		BaseURL:        server.URL,
		ClientID:       megafloydauth.DefaultClientID,
		Platform:       megafloydauth.DefaultPlatform,
		Version:        "test",
		HomeDir:        dir,
		CredentialsDir: dir,
		HTTPClient:     server.Client(),
		Now:            func() time.Time { return now },
	}
	store := megafloydauth.FileStore{Dir: dir}
	const name = "test-slot"
	require.NoError(t, store.Save(name, megafloydauth.Token{
		AccessToken:  "stale-access",
		RefreshToken: "stale-refresh",
		ExpiresAt:    now.Add(-time.Hour).Unix(),
		ExpiresIn:    3600,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
	}))
	manager := &megafloydauth.Manager{
		Flow:  megafloydauth.NewClient(authCfg),
		Store: store,
		Name:  name,
		Now:   func() time.Time { return now },
	}

	source := &managerTokenSource{manager: manager}
	useCompanyTokenSource(t, source)
	useCompanyExecutable(t, "/opt/floyd")
	configStore := config.NewTestStore(companyAuthConfig())

	require.NoError(t, applyCompanyAuthCredentials(t.Context(), configStore))

	got, _ := configStore.Config().Providers.Get("company")
	require.Equal(t, "fresh-access", got.APIKey)

	mu.Lock()
	require.Len(t, grants, 1, "the expired token should refresh exactly once")
	grant := grants[0]
	mu.Unlock()
	require.Equal(t, "refresh_token", grant.Get("grant_type"))
	require.Equal(t, "stale-refresh", grant.Get("refresh_token"))
	require.Equal(t, megafloydauth.DefaultClientID, grant.Get("client_id"))

	// A second application reuses the refreshed token without another
	// network call.
	require.NoError(t, applyCompanyAuthCredentials(t.Context(), configStore))
	mu.Lock()
	require.Len(t, grants, 1)
	mu.Unlock()
}

// managerTokenSource adapts Manager to the provider wiring interface while
// counting calls.
type managerTokenSource struct {
	manager *megafloydauth.Manager
	calls   int
}

func (s *managerTokenSource) AccessToken(ctx context.Context) (string, error) {
	s.calls++
	return s.manager.AccessToken(ctx)
}

var (
	_ companyTokenSource = (*fakeCompanyTokenSource)(nil)
	_ companyTokenSource = (*managerTokenSource)(nil)
)

func TestCanonicalUnusedAccountDoesNotBlockBeam(t *testing.T) {
	for _, withoutSelection := range []bool{false, true} {
		t.Run(fmt.Sprint(withoutSelection), func(t *testing.T) {
			source := &fakeCompanyTokenSource{err: megafloydauth.ErrNotLoggedIn}
			useCompanyTokenSource(t, source)
			cfg := companyAuthConfig()
			selected := config.SelectedModel{Provider: "opencode-go", Model: "beam-free", MaxTokens: 131072, ReasoningEffort: "medium"}
			cfg.Models = map[config.SelectedModelType]config.SelectedModel{config.SelectedModelTypeLarge: selected}
			cfg.Providers.Set("opencode-go", config.ProviderConfig{ID: "opencode-go", APIKey: "unchanged-beam-fixture"})
			if withoutSelection {
				cfg.Models = nil
			}
			require.NoError(t, applyCompanyAuthCredentials(t.Context(), config.NewTestStore(cfg)))
			require.Zero(t, source.calls, "an unused logged-out account must not be consulted")
			beam, _ := cfg.Providers.Get("opencode-go")
			require.Equal(t, "unchanged-beam-fixture", beam.APIKey)
			company, _ := cfg.Providers.Get("company")
			require.Equal(t, config.CompanyAuthAPIKeyScheme+"default", company.APIKey)
			if !withoutSelection {
				require.Equal(t, selected, cfg.Models[config.SelectedModelTypeLarge])
			}
		})
	}
}
