package megafloydauth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCredentialKeyMatchesLiveSlot(t *testing.T) {
	t.Parallel()

	const host = "https://inference.tail58d565.ts.net:8446"
	// The live f7 token file is floyd-code-env-e45bd67489796b47.json.
	require.Equal(t, "oauth/floyd-code-env-e45bd67489796b47", CredentialKey(host, host))
	require.Equal(t, "oauth/floyd-code-env-e45bd67489796b47", CredentialKey(host+"/", host+"/"))
	require.Equal(t, "oauth/floyd-code", CredentialKey("", ""))
}

func TestStorageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key     string
		want    string
		wantErr bool
	}{
		{key: "oauth/floyd-code", want: "floyd-code"},
		{key: "oauth/floyd-code-env-e45bd67489796b47", want: "floyd-code-env-e45bd67489796b47"},
		{key: "floyd-code", want: "floyd-code"},
		{key: "", want: "floyd-code"},
		{key: "oauth/", wantErr: true},
		{key: "oauth/../escape", wantErr: true},
		{key: "../escape", wantErr: true},
	}
	for _, tt := range tests {
		got, err := StorageName(tt.key)
		if tt.wantErr {
			require.Error(t, err, tt.key)
			continue
		}
		require.NoError(t, err, tt.key)
		require.Equal(t, tt.want, got, tt.key)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := FileStore{Dir: t.TempDir()}
	want := Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    1794070096,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
		ExpiresIn:    2592000,
	}
	require.NoError(t, store.Save("floyd-code-env-test", want))

	got, ok, err := store.Load("floyd-code-env-test")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(store.Dir, "floyd-code-env-test.json"))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	names, err := store.List()
	require.NoError(t, err)
	require.Equal(t, []string{"floyd-code-env-test"}, names)

	require.NoError(t, store.Remove("floyd-code-env-test"))
	_, ok, err = store.Load("floyd-code-env-test")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestFileStoreRejectsTraversal(t *testing.T) {
	t.Parallel()

	store := FileStore{Dir: t.TempDir()}
	require.Error(t, store.Save("../escape", Token{}))
	require.Error(t, store.Save(".hidden", Token{}))
}

func TestFromEnvPrecedence(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"MEGAFLOYD_AUTH_HOST":      "https://mega.example",
		"FLOYD_CODE_OAUTH_HOST":    "https://f7.example",
		"FLOYD_CODE_BASE_URL":      "https://f7.example/api",
		"MEGAFLOYD_HOME":           "/tmp/megafloyd-home",
		"MEGAFLOYD_VERSION":        "1.2.3",
		"MEGAFLOYD_AUTH_CLIENT_ID": "client-from-env",
	}
	cfg := FromEnv(func(key string) string { return env[key] })

	require.Equal(t, "https://mega.example", cfg.OAuthHost)
	require.Equal(t, "https://f7.example/api", cfg.BaseURL)
	require.Equal(t, "client-from-env", cfg.ClientID)
	require.Equal(t, "1.2.3", cfg.Version)
	require.Equal(t, "/tmp/megafloyd-home", cfg.HomeDir)
	require.Equal(t, "/tmp/megafloyd-home/credentials", cfg.CredentialsPath())
}

func TestTokenExpiryUsesHalfLife(t *testing.T) {
	t.Parallel()

	token := Token{AccessToken: "a", ExpiresIn: 2592000, ExpiresAt: 1794070096}
	expiry := time.Unix(token.ExpiresAt, 0)

	require.False(t, token.Expired(expiry.Add(-20*24*time.Hour)))
	require.True(t, token.Expired(expiry.Add(-10*24*time.Hour)))
	require.True(t, token.Expired(expiry))
}

func TestHeadersMintDeviceID(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cfg := Config{HomeDir: home, Platform: DefaultPlatform, Version: "9.9.9"}
	headers, err := cfg.Headers()
	require.NoError(t, err)
	require.Equal(t, DefaultPlatform, headers["X-Msh-Platform"])
	require.Equal(t, "9.9.9", headers["X-Msh-Version"])
	require.NotEmpty(t, headers["X-Msh-Device-Id"])

	data, err := os.ReadFile(filepath.Join(home, DeviceIDFile))
	require.NoError(t, err)
	require.Equal(t, headers["X-Msh-Device-Id"], string(data))

	again, err := cfg.Headers()
	require.NoError(t, err)
	require.Equal(t, headers["X-Msh-Device-Id"], again["X-Msh-Device-Id"])
}

func TestUnimplementedFlowFailsClosed(t *testing.T) {
	t.Parallel()

	var flow DeviceFlow = UnimplementedFlow{}
	_, err := flow.RequestDeviceAuthorization(t.Context())
	require.ErrorIs(t, err, ErrNotImplemented)
}
