package megafloydauth

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultClientID is the public device-code client registered with the
	// floyd-auth server. Verified in the donor
	// packages/oauth/src/constants.ts.
	DefaultClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"

	// DefaultPlatform is the X-Msh-Platform value the f7 CLI reports.
	// A distinct megafloyd value has not been tested against the server,
	// so the known-good value is kept.
	DefaultPlatform = "floyd_code_cli"

	// DefaultPollInterval is the polling interval used when the server
	// omits one (donor default).
	DefaultPollInterval = 5 * time.Second

	// DeviceCodeTimeout bounds one login attempt (donor default).
	DeviceCodeTimeout = 15 * time.Minute

	// MinRefreshThreshold is the floor for proactive refresh. The donor
	// refreshes at half the token lifetime, never later than this.
	MinRefreshThreshold = 5 * time.Minute

	// RefreshThresholdRatio is the donor's proactive-refresh fraction.
	RefreshThresholdRatio = 0.5
)

// Config is the resolved floyd-auth configuration for one process.
type Config struct {
	// OAuthHost is the device-code and token endpoint base, without a
	// trailing slash.
	OAuthHost string
	// BaseURL is the managed API base used for /models, /me and /usages.
	BaseURL string
	// ClientID is the public device-flow client identifier.
	ClientID string
	// Platform is reported in X-Msh-Platform.
	Platform string
	// Version is reported in X-Msh-Version and the User-Agent.
	Version string
	// HomeDir is the megafloyd home directory.
	HomeDir string
	// CredentialsDir overrides <HomeDir>/credentials when set.
	CredentialsDir string
	// DeviceAuthorizationPath overrides the device-authorization endpoint
	// path. Empty means DefaultDeviceAuthorizationPath.
	DeviceAuthorizationPath string
	// TokenPath overrides the token endpoint path. Empty means
	// DefaultTokenPath. Both the device grant and refresh use it.
	TokenPath string
	// HTTPClient is injectable for tests. Nil means the caller's default.
	HTTPClient HTTPDoer
	// Now is injectable for tests. Nil means time.Now.
	Now func() time.Time
	// Sleep is injectable for tests. Nil means a context-aware sleep.
	Sleep func(ctx context.Context, d time.Duration) error
}

// EnvLookup matches os.Getenv. Tests pass a map-backed lookup.
type EnvLookup func(string) string

// FromEnv resolves a Config. A nil lookup uses os.Getenv.
//
// MEGAFLOYD_* names win over the FLOYD_CODE_* mirrors that the f7 launcher
// exports, so an existing f7 environment works unchanged.
func FromEnv(lookup EnvLookup) Config {
	if lookup == nil {
		lookup = os.Getenv
	}
	return Config{
		OAuthHost: firstNonEmpty(
			lookup("MEGAFLOYD_AUTH_HOST"),
			lookup("FLOYD_CODE_OAUTH_HOST"),
			lookup("FLOYD_OAUTH_HOST"),
		),
		BaseURL: firstNonEmpty(
			lookup("MEGAFLOYD_AUTH_BASE_URL"),
			lookup("FLOYD_CODE_BASE_URL"),
		),
		ClientID: firstNonEmpty(lookup("MEGAFLOYD_AUTH_CLIENT_ID"), DefaultClientID),
		Platform: firstNonEmpty(lookup("MEGAFLOYD_AUTH_PLATFORM"), DefaultPlatform),
		Version:  firstNonEmpty(lookup("MEGAFLOYD_VERSION"), lookup("FLOYD_CODE_VERSION")),
		HomeDir:  firstNonEmpty(lookup("MEGAFLOYD_HOME"), defaultHomeDir()),
		DeviceAuthorizationPath: firstNonEmpty(
			lookup("MEGAFLOYD_AUTH_DEVICE_PATH"),
		),
		TokenPath: firstNonEmpty(lookup("MEGAFLOYD_AUTH_TOKEN_PATH")),
	}
}

// clientID returns the configured client ID or the built-in default.
func (c Config) clientID() string {
	if c.ClientID != "" {
		return c.ClientID
	}
	return DefaultClientID
}

// CredentialsPath returns the directory that holds token files, or the
// empty string when no home directory could be resolved.
func (c Config) CredentialsPath() string {
	if c.CredentialsDir != "" {
		return c.CredentialsDir
	}
	if c.HomeDir == "" {
		return ""
	}
	return filepath.Join(c.HomeDir, "credentials")
}

// NowFunc returns the configured clock.
func (c Config) NowFunc() func() time.Time {
	if c.Now != nil {
		return c.Now
	}
	return time.Now
}

// SleepFunc returns the configured sleeper.
func (c Config) SleepFunc() func(context.Context, time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep
	}
	return sleepContext
}

func defaultHomeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, ".megafloyd")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
