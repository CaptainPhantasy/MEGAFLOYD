package config

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/catwalk/pkg/embedded"
	"charm.land/fantasy/providers/anthropic"
	"github.com/charmbracelet/x/etag"
	"github.com/legacy-ai/floyd/internal/agent/hyper"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/home"
	"github.com/legacy-ai/floyd/internal/megafloyd"
)

type syncer[T any] interface {
	Get(context.Context) (T, error)
}

var (
	providerOnce sync.Once
	providerList []catwalk.Provider
	providerErr  error
)

// file to cache provider data
func cachePathFor(name string) string {
	if megafloyd.IsActive() {
		// Provider catalogs follow the same account-owned cache policy as the
		// rest of MegaFloyd, including explicit MEGAFLOYD_CACHE_DIR overrides.
		if dir := GlobalCacheDir(); dir != "" {
			return filepath.Join(dir, name+".json")
		}
		return ""
	}

	xdgDataHome := os.Getenv("XDG_DATA_HOME")
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, appName, name+".json")
	}

	// return the path to the main data directory
	// for windows, it should be in `%LOCALAPPDATA%/floyd/`
	// for linux and macOS, it should be in `$HOME/.local/share/floyd/`
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(localAppData, appName, name+".json")
	}

	return filepath.Join(home.Dir(), ".local", "share", appName, name+".json")
}

// UpdateProviders updates the Catwalk providers list from a specified source.
func UpdateProviders(pathOrURL string) error {
	var providers []catwalk.Provider
	pathOrURL = cmp.Or(pathOrURL, os.Getenv("CATWALK_URL"), defaultCatwalkURL)

	switch {
	case pathOrURL == "embedded":
		providers = embedded.GetAll()
	case strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://"):
		var err error
		providers, err = catwalk.NewWithURL(pathOrURL).GetProviders(context.Background(), "")
		if err != nil {
			return fmt.Errorf("failed to fetch providers from Catwalk: %w", err)
		}
	default:
		content, err := os.ReadFile(pathOrURL)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		if err := json.Unmarshal(content, &providers); err != nil {
			return fmt.Errorf("failed to unmarshal provider data: %w", err)
		}
		if len(providers) == 0 {
			return fmt.Errorf("no providers found in the provided source")
		}
	}

	if err := newCache[[]catwalk.Provider](cachePathFor("providers")).Store(providers); err != nil {
		return fmt.Errorf("failed to save providers to cache: %w", err)
	}

	slog.Info("Providers updated successfully", "count", len(providers), "from", pathOrURL, "to", cachePathFor)
	return nil
}

// resolveHyperAPIKey returns the Hyper API key from the environment or
// the raw config value. The env var takes precedence.
func resolveHyperAPIKey(cfg *Config) string {
	if key := os.Getenv("HYPER_API_KEY"); key != "" {
		return key
	}
	if cfg == nil || cfg.Providers == nil {
		return ""
	}
	pc, ok := cfg.Providers.Get("hyper")
	if !ok {
		return ""
	}
	return pc.APIKey
}

// HyperTokenRefresher is a function that refreshes the Hyper OAuth
// token. It is passed to Providers so the catalog fetch can retry on
// 401 without relying on package-global state.
type HyperTokenRefresher func(context.Context) error

// UpdateHyper updates the Hyper provider information from a specified URL.
func UpdateHyper(pathOrURL string) error {
	var provider catwalk.Provider
	pathOrURL = cmp.Or(pathOrURL, hyper.BaseURL())

	switch {
	case pathOrURL == "embedded":
		provider = hyper.Embedded()
	case strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://"):
		client := realHyperClient{
			baseURL:    pathOrURL,
			resolveKey: func() string { return resolveHyperAPIKey(nil) },
		}
		var err error
		provider, err = client.Get(context.Background(), "")
		if err != nil {
			return fmt.Errorf("failed to fetch provider from Hyper: %w", err)
		}
	default:
		content, err := os.ReadFile(pathOrURL)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		if err := json.Unmarshal(content, &provider); err != nil {
			return fmt.Errorf("failed to unmarshal provider data: %w", err)
		}
	}

	if err := newCache[catwalk.Provider](cachePathFor("hyper")).Store(provider); err != nil {
		return fmt.Errorf("failed to save Hyper provider to cache: %w", err)
	}

	slog.Info("Hyper provider updated successfully", "from", pathOrURL, "to", cachePathFor("hyper"))
	return nil
}

var (
	catwalkSyncer = &catwalkSync{}
	hyperSyncer   = &hyperSync{}
)

// Providers returns the list of providers, taking into account cached results
// and whether or not auto update is enabled.
//
// It will:
// 1. if auto update is disabled, it'll return the embedded providers at the
// time of release.
// 2. load the cached providers
// 3. try to get the fresh list of providers, and return either this new list,
// the cached list, or the embedded list if all others fail.
//
// A returned error is advisory: it reports that the catalog could not be
// cached, or that an upstream returned nothing usable. It never means that no
// providers are available, so callers should surface it as a warning and keep
// using the returned list. A refresh that simply could not reach the network
// is not an error at all: the cached or embedded catalog is a sound answer, so
// those are logged and the fallback is returned.
func Providers(cfg *Config, opts ...HyperTokenRefresher) ([]catwalk.Provider, error) {
	providerOnce.Do(func() {
		var wg sync.WaitGroup
		providers := csync.NewSlice[catwalk.Provider]()
		autoupdate := !cfg.Options.DisableProviderAutoUpdate
		customProvidersOnly := cfg.Options.DisableDefaultProviders

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		// Each goroutine owns its own error so the two can report
		// independently without racing on a shared slice.
		var catwalkErr, hyperErr error
		var hyperProvider catwalk.Provider

		wg.Go(func() {
			if customProvidersOnly {
				return
			}
			catwalkURL := cmp.Or(os.Getenv("CATWALK_URL"), defaultCatwalkURL)
			client := catwalk.NewWithURL(catwalkURL)
			path := cachePathFor("providers")
			catwalkSyncer.Init(client, path, autoupdate)

			// A failure to refresh or cache the catalog is worth
			// reporting, but the syncer still hands back the cached or
			// embedded list. Dropping that would leave the user with no
			// providers at all over a transient disk or network problem.
			items, err := catwalkSyncer.Get(ctx)
			if err != nil {
				catwalkURL := fmt.Sprintf("%s/v2/providers", cmp.Or(os.Getenv("CATWALK_URL"), defaultCatwalkURL))
				catwalkErr = fmt.Errorf("MegaFloyd was unable to fetch an updated list of providers from %s. Consider setting FLOYD_DISABLE_PROVIDER_AUTO_UPDATE=1 to use the embedded providers bundled at the time of this MegaFloyd release. You can also update providers manually. For more info see megafloyd update-providers --help.\n\nCause: %w", catwalkURL, err) //nolint:staticcheck
			}
			providers.Append(items...)
		})

		wg.Go(func() {
			if customProvidersOnly {
				return
			}
			path := cachePathFor("hyper")
			cfgSnapshot := cfg
			var refresher func(context.Context) error
			if len(opts) > 0 {
				refresher = opts[0]
			}
			hyperSyncer.Init(realHyperClient{
				baseURL:      hyper.BaseURL(),
				resolveKey:   func() string { return resolveHyperAPIKey(cfgSnapshot) },
				refreshToken: refresher,
			}, path, autoupdate)

			// As above: keep whatever provider we were handed. The syncer
			// already falls back to the cached or embedded copy, so an
			// error here means "could not refresh", not "no Hyper". This
			// matters more than for other providers because Hyper's
			// endpoint and model list live in the catalog rather than in
			// the user's config: dropping it signs a logged-in user out.
			item, err := hyperSyncer.Get(ctx)
			if err != nil {
				hyperErr = fmt.Errorf("MegaFloyd was unable to fetch updated information from Hyper: %w", err) //nolint:staticcheck
			}
			hyperProvider = item
		})

		wg.Wait()

		if hyperProvider.ID != "" {
			providerList = append([]catwalk.Provider{hyperProvider}, slices.Collect(providers.Seq())...)
		} else {
			providerList = slices.Collect(providers.Seq())
		}
		providerErr = errors.Join(catwalkErr, hyperErr)
	})
	return providerList, providerErr
}

// UpdateProviderInList replaces a provider in the memoized provider list
// returned by Providers(). This is used after re-fetching a single
// provider (e.g. Hyper after OAuth) so that all callers of Providers()
// see the updated entry without needing to reset sync.Once.
func UpdateProviderInList(provider catwalk.Provider) {
	for i, p := range providerList {
		if p.ID == provider.ID {
			providerList[i] = provider
			return
		}
	}
	// Provider not found in list; prepend it.
	providerList = append([]catwalk.Provider{provider}, providerList...)
}

type cache[T any] struct {
	path string
}

func newCache[T any](path string) cache[T] {
	return cache[T]{path: path}
}

func (c cache[T]) Get() (T, string, error) {
	var v T
	data, err := os.ReadFile(c.path)
	if err != nil {
		return v, "", fmt.Errorf("failed to read provider cache file: %w", err)
	}

	if err := json.Unmarshal(data, &v); err != nil {
		return v, "", fmt.Errorf("failed to unmarshal provider data from cache: %w", err)
	}

	return v, etag.Of(data), nil
}

func (c cache[T]) Store(v T) error {
	slog.Info("Saving provider data to disk", "path", c.path)
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for provider cache: %w", err)
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal provider data: %w", err)
	}

	// Written through a temporary file and renamed into place. Several MegaFloyd
	// instances start independently and race to refresh this cache, and a
	// truncating write would let one of them read a half-written catalog and
	// silently fall back to the bundled copy.
	if err := atomicWriteFile(c.path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write provider data to cache: %w", err)
	}
	return nil
}

// CompanyAuthAPIKeyScheme marks a provider api_key as a placeholder for a
// company-auth access token. Set it in provider configuration, for example
// api_key = "company-auth://default", then run `floyd auth login` once. The
// application replaces the placeholder with a bearer token in memory before
// providers are built; the placeholder is never sent to a provider and the
// token is never written back to the config file. Use it with provider types
// that carry a bearer credential (the OpenAI-compatible default and
// anthropic); other wire protocols use different credential fields.
//
// Model discovery runs during config load, before the application can
// substitute the token. Give a placeholder provider an explicit models list
// or set discover_models = false so the placeholder is never sent to the
// provider's /models endpoint.
const CompanyAuthAPIKeyScheme = "company-auth://"

// IsCompanyAuthAPIKey reports whether value is a company-auth placeholder.
func IsCompanyAuthAPIKey(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), CompanyAuthAPIKeyScheme)
}

// CompanyAuthProviderIDs returns the sorted IDs of providers configured
// with a company-auth placeholder. A provider is included when either its
// resolved key or its original template carries the placeholder.
func CompanyAuthProviderIDs(cfg *Config) []string {
	if cfg == nil || cfg.Providers == nil {
		return nil
	}
	var ids []string
	for id, provider := range cfg.Providers.Copy() {
		if provider.Disable {
			continue
		}
		if IsCompanyAuthAPIKey(provider.APIKey) || IsCompanyAuthAPIKey(provider.APIKeyTemplate) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// CompanyAuthRefreshCommand returns the api_key template that re-resolves a
// company-auth token after a provider rejects the current one with 401. The
// coordinator re-runs this template through the shell resolver, which calls
// `auth token` and therefore Manager.Token's lazy refresh, then rebuilds the
// provider.
func CompanyAuthRefreshCommand(executable string) string {
	executable = strings.TrimSpace(executable)
	if executable == "" {
		return ""
	}
	return "$(" + shellSingleQuote(executable) + " auth token)"
}

// SetProviderCompanyAuthCredential replaces a company-auth placeholder with
// accessToken and installs refreshCommand as the provider's api_key
// template. The change is in memory only: credentials are never persisted to
// the user's config file. It reports false when the provider does not exist
// or does not currently carry a placeholder, so a real API-key provider can
// never be overwritten by this path.
func (s *ConfigStore) SetProviderCompanyAuthCredential(
	providerID, accessToken, refreshCommand string,
) bool {
	if s == nil || accessToken == "" || providerID == "" {
		return false
	}
	cfg := s.Config()
	if cfg == nil || cfg.Providers == nil {
		return false
	}
	current, ok := cfg.Providers.Get(providerID)
	if !ok || !IsCompanyAuthAPIKey(current.APIKey) {
		return false
	}

	applied := false
	s.mutateInMemory(func(cfg *Config) {
		if cfg.Providers == nil {
			return
		}
		provider, ok := cfg.Providers.Get(providerID)
		if !ok || !IsCompanyAuthAPIKey(provider.APIKey) {
			return
		}

		// cloneForWrite shares the provider map with the published
		// snapshot, so replace it instead of mutating it in place.
		providers := csync.NewMap[string, ProviderConfig]()
		for id, pc := range cfg.Providers.Copy() {
			providers.Set(id, pc)
		}
		provider.APIKey = companyAuthBearerToken(provider.Type, accessToken)
		provider.APIKeyTemplate = refreshCommand
		providers.Set(providerID, provider)
		cfg.Providers = providers
		applied = true
	})
	return applied
}

// companyAuthBearerToken renders a token for the provider's wire protocol.
// Anthropic-style providers read the Authorization header only when the key
// is already bearer-prefixed; every other supported type adds the prefix
// itself.
func companyAuthBearerToken(providerType catwalk.Type, accessToken string) string {
	if providerType == anthropic.Name {
		return "Bearer " + accessToken
	}
	return accessToken
}

// shellSingleQuote quotes a value for safe use inside a POSIX shell command
// substitution.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
