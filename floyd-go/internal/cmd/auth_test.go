package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// fakeAuthFlow is a DeviceFlow with scripted responses.
type fakeAuthFlow struct {
	mu        sync.Mutex
	auth      megafloydauth.DeviceAuthorization
	polls     []megafloydauth.PollResult
	pollIndex int
	refresh   *megafloydauth.Token
}

func (f *fakeAuthFlow) RequestDeviceAuthorization(context.Context) (megafloydauth.DeviceAuthorization, error) {
	return f.auth, nil
}

func (f *fakeAuthFlow) PollDeviceToken(context.Context, string) (megafloydauth.PollResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pollIndex >= len(f.polls) {
		return megafloydauth.PollResult{
			Status:    megafloydauth.PollPending,
			ErrorCode: "authorization_pending",
		}, nil
	}
	result := f.polls[f.pollIndex]
	f.pollIndex++
	return result, nil
}

func (f *fakeAuthFlow) RefreshAccessToken(context.Context, string) (megafloydauth.Token, error) {
	if f.refresh != nil {
		return *f.refresh, nil
	}
	return megafloydauth.Token{}, errors.New("fakeAuthFlow: refresh not scripted")
}

func newTestManager(
	t *testing.T, flow megafloydauth.DeviceFlow, now time.Time,
) (*megafloydauth.Manager, megafloydauth.FileStore, string) {
	t.Helper()
	store := megafloydauth.FileStore{Dir: t.TempDir()}
	const name = "floyd-code-env-test"
	manager := &megafloydauth.Manager{
		Flow:  flow,
		Store: store,
		Name:  name,
		Now:   func() time.Time { return now },
	}
	return manager, store, name
}

func useAuthRuntime(t *testing.T, runtime *authRuntime) {
	t.Helper()
	previous := authRuntimeFactory
	authRuntimeFactory = func() (*authRuntime, error) { return runtime, nil }
	t.Cleanup(func() { authRuntimeFactory = previous })
}

func runAuthCommand(
	t *testing.T, cmd *cobra.Command, run func(*cobra.Command, []string) error,
) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	t.Cleanup(func() {
		cmd.SetOut(nil)
		cmd.SetErr(nil)
		cmd.SetContext(context.Background())
	})
	err := run(cmd, nil)
	return out.String(), err
}

func TestAuthCommandRegistration(t *testing.T) {
	t.Parallel()

	// authCmd must win over loginCmd's legacy "auth" alias.
	cmd, _, err := rootCmd.Find([]string{"auth"})
	require.NoError(t, err)
	require.Same(t, authCmd, cmd)
	require.Equal(t, "auth", cmd.Name())

	for _, tt := range []struct {
		path []string
		want *cobra.Command
	}{
		{path: []string{"auth", "login"}, want: authLoginCmd},
		{path: []string{"auth", "logout"}, want: authLogoutCmd},
		{path: []string{"auth", "status"}, want: authStatusCmd},
		{path: []string{"auth", "token"}, want: authTokenCmd},
	} {
		cmd, _, err := rootCmd.Find(tt.path)
		require.NoError(t, err, tt.path)
		require.Same(t, tt.want, cmd, tt.path)
	}
}

func TestAuthHelpExitsZero(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	for _, args := range [][]string{
		{"auth", "--help"},
		{"auth", "login", "--help"},
		{"auth", "logout", "--help"},
		{"auth", "status", "--help"},
	} {
		var out bytes.Buffer
		rootCmd.SetArgs(args)
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		require.NoError(t, rootCmd.Execute(), args)
		require.Contains(t, out.String(), "Usage:", args)
	}
}

func TestAuthLoginStoresToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := megafloydauth.Token{
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		ExpiresAt:    now.Add(time.Hour).Unix(),
		ExpiresIn:    3600,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
	}
	flow := &fakeAuthFlow{
		auth: megafloydauth.DeviceAuthorization{
			UserCode:                "ABCD-EFGH",
			DeviceCode:              "device-1",
			VerificationURIComplete: "https://auth.example.test/approve?user_code=ABCD-EFGH",
			Interval:                time.Second,
		},
		polls: []megafloydauth.PollResult{{Status: megafloydauth.PollSuccess, Token: token}},
	}
	manager, store, name := newTestManager(t, flow, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	useAuthRuntime(t, &authRuntime{
		Manager:   manager,
		Host:      "https://auth.example.test",
		StorePath: path,
	})

	out, err := runAuthCommand(t, authLoginCmd, runAuthLogin)
	require.NoError(t, err)
	require.Contains(t, out, "https://auth.example.test/approve?user_code=ABCD-EFGH")
	require.Contains(t, out, "ABCD-EFGH")
	require.Contains(t, out, "Logged in")
	require.NotContains(t, out, "access-secret")
	require.NotContains(t, out, "refresh-secret")

	stored, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, token, stored)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestAuthLoginAccessDenied(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	flow := &fakeAuthFlow{
		auth:  megafloydauth.DeviceAuthorization{DeviceCode: "device-1"},
		polls: []megafloydauth.PollResult{{Status: megafloydauth.PollDenied}},
	}
	manager, store, name := newTestManager(t, flow, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	_, err = runAuthCommand(t, authLoginCmd, runAuthLogin)
	require.ErrorIs(t, err, megafloydauth.ErrAccessDenied)
}

func TestAuthStatusLoggedIn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	require.NoError(t, store.Save(name, megafloydauth.Token{
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		ExpiresAt:    now.Add(time.Hour).Unix(),
		ExpiresIn:    3600,
		Scope:        "floyd-code",
		TokenType:    "Bearer",
	}))
	useAuthRuntime(t, &authRuntime{
		Manager:   manager,
		Host:      "https://auth.example.test",
		StorePath: path,
	})

	out, err := runAuthCommand(t, authStatusCmd, runAuthStatus)
	require.NoError(t, err)
	require.Contains(t, out, "Logged in to https://auth.example.test")
	require.Contains(t, out, "Expires:")
	require.Contains(t, out, "Refresh due: no")
	require.Contains(t, out, "Scope: floyd-code")
	require.NotContains(t, out, "access-secret")
	require.NotContains(t, out, "refresh-secret")
}

func TestAuthStatusRefreshDue(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	require.NoError(t, store.Save(name, megafloydauth.Token{
		AccessToken: "access",
		ExpiresAt:   now.Add(10 * time.Minute).Unix(),
		ExpiresIn:   3600,
	}))
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	out, err := runAuthCommand(t, authStatusCmd, runAuthStatus)
	require.NoError(t, err)
	require.Contains(t, out, "Refresh due: yes")
}

func TestAuthStatusNotLoggedIn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	_, err = runAuthCommand(t, authStatusCmd, runAuthStatus)
	require.ErrorIs(t, err, megafloydauth.ErrNotLoggedIn)
	require.Contains(t, err.Error(), "auth login")
}

func TestAuthStatusRevoked(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	require.NoError(t, store.Save(name, megafloydauth.Token{Scope: "floyd-code"}))
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	_, err = runAuthCommand(t, authStatusCmd, runAuthStatus)
	require.ErrorIs(t, err, megafloydauth.ErrLoginRevoked)
}

func TestAuthLogoutRemovesToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	require.NoError(t, store.Save(name, megafloydauth.Token{AccessToken: "access"}))
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	out, err := runAuthCommand(t, authLogoutCmd, runAuthLogout)
	require.NoError(t, err)
	require.Contains(t, out, "Logged out")
	_, ok, err := store.Load(name)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAuthLogoutWhenNotLoggedIn(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	path, err := store.Path(name)
	require.NoError(t, err)
	useAuthRuntime(t, &authRuntime{Manager: manager, StorePath: path})

	out, err := runAuthCommand(t, authLogoutCmd, runAuthLogout)
	require.NoError(t, err)
	require.Contains(t, out, "Not logged in")
}

func TestAuthTokenPrintsStoredAccessToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	manager, store, name := newTestManager(t, &fakeAuthFlow{}, now)
	require.NoError(t, store.Save(name, megafloydauth.Token{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    now.Add(time.Hour).Unix(),
		ExpiresIn:    3600,
	}))
	useAuthRuntime(t, &authRuntime{Manager: manager})

	out, err := runAuthCommand(t, authTokenCmd, runAuthToken)
	require.NoError(t, err)
	require.Equal(t, "access-token\n", out)
}

func TestAuthTokenRefreshesExpiredToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	flow := &fakeAuthFlow{refresh: &megafloydauth.Token{
		AccessToken:  "refreshed-access",
		RefreshToken: "refreshed-refresh",
		ExpiresAt:    now.Add(time.Hour).Unix(),
		ExpiresIn:    3600,
	}}
	manager, store, name := newTestManager(t, flow, now)
	require.NoError(t, store.Save(name, megafloydauth.Token{
		AccessToken:  "expired-access",
		RefreshToken: "expired-refresh",
		ExpiresAt:    now.Add(-time.Hour).Unix(),
		ExpiresIn:    3600,
	}))
	useAuthRuntime(t, &authRuntime{Manager: manager})

	out, err := runAuthCommand(t, authTokenCmd, runAuthToken)
	require.NoError(t, err)
	require.Equal(t, "refreshed-access\n", out)

	stored, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "refreshed-access", stored.AccessToken)
}

func TestAuthTokenRequiresLogin(t *testing.T) {
	manager, _, _ := newTestManager(t, &fakeAuthFlow{}, time.Unix(1_700_000_000, 0))
	useAuthRuntime(t, &authRuntime{Manager: manager})

	_, err := runAuthCommand(t, authTokenCmd, runAuthToken)
	require.ErrorIs(t, err, megafloydauth.ErrNotLoggedIn)
}

func TestAuthLoginOverHTTP(t *testing.T) {
	var (
		mu   sync.Mutex
		seen = map[string]int{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case megafloydauth.DefaultDeviceAuthorizationPath:
			_, _ = io.WriteString(w, `{
				"user_code": "WXYZ",
				"device_code": "device-http",
				"verification_uri": "https://verify.example.test/approve",
				"verification_uri_complete": "https://verify.example.test/approve?user_code=WXYZ",
				"interval": 1
			}`)
		case megafloydauth.DefaultTokenPath:
			_, _ = io.WriteString(w, `{
				"access_token": "access-http",
				"refresh_token": "refresh-http",
				"expires_in": 3600,
				"scope": "floyd-code",
				"token_type": "Bearer"
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	home := t.TempDir()
	t.Setenv("MEGAFLOYD_AUTH_HOST", server.URL)
	t.Setenv("MEGAFLOYD_AUTH_BASE_URL", server.URL)
	t.Setenv("MEGAFLOYD_AUTH_CLIENT_ID", "")
	t.Setenv("MEGAFLOYD_AUTH_DEVICE_PATH", "")
	t.Setenv("MEGAFLOYD_AUTH_TOKEN_PATH", "")
	t.Setenv("MEGAFLOYD_HOME", home)
	t.Setenv("MEGAFLOYD_VERSION", "9.9.9")
	t.Setenv("FLOYD_CODE_OAUTH_HOST", "")
	t.Setenv("FLOYD_CODE_BASE_URL", "")
	t.Setenv("FLOYD_OAUTH_HOST", "")

	runtime, err := buildAuthRuntime()
	require.NoError(t, err)
	useAuthRuntime(t, runtime)

	out, err := runAuthCommand(t, authLoginCmd, runAuthLogin)
	require.NoError(t, err)
	require.Contains(t, out, "https://verify.example.test/approve?user_code=WXYZ")
	require.NotContains(t, out, "access-http")
	require.NotContains(t, out, "refresh-http")

	mu.Lock()
	require.Equal(t, 1, seen[megafloydauth.DefaultDeviceAuthorizationPath])
	require.Equal(t, 1, seen[megafloydauth.DefaultTokenPath])
	mu.Unlock()

	require.FileExists(t, runtime.StorePath)
	store := megafloydauth.FileStore{Dir: filepath.Dir(runtime.StorePath)}
	name := strings.TrimSuffix(filepath.Base(runtime.StorePath), ".json")
	token, ok, err := store.Load(name)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "access-http", token.AccessToken)
	require.Equal(t, "refresh-http", token.RefreshToken)
}

func TestBuildAuthRuntimeFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MEGAFLOYD_AUTH_HOST", "https://auth.example.test")
	t.Setenv("MEGAFLOYD_AUTH_BASE_URL", "https://api.example.test")
	t.Setenv("MEGAFLOYD_HOME", home)
	t.Setenv("MEGAFLOYD_VERSION", "9.9.9")
	t.Setenv("MEGAFLOYD_AUTH_DEVICE_PATH", "")
	t.Setenv("MEGAFLOYD_AUTH_TOKEN_PATH", "")
	t.Setenv("FLOYD_CODE_OAUTH_HOST", "")
	t.Setenv("FLOYD_CODE_BASE_URL", "")
	t.Setenv("FLOYD_OAUTH_HOST", "")

	runtime, err := buildAuthRuntime()
	require.NoError(t, err)
	require.Equal(t, "https://auth.example.test", runtime.Host)

	name, err := megafloydauth.StorageName(
		megafloydauth.CredentialKey("https://auth.example.test", "https://api.example.test"),
	)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "credentials", name+".json"), runtime.StorePath)

	client, ok := runtime.Manager.Flow.(*megafloydauth.Client)
	require.True(t, ok)
	require.Equal(t, megafloydauth.DefaultClientID, client.Config.ClientID)
	require.Equal(t, "9.9.9", client.Config.Version)
}

func TestBuildAuthRuntimeRequiresHost(t *testing.T) {
	t.Setenv("MEGAFLOYD_AUTH_HOST", "")
	t.Setenv("FLOYD_CODE_OAUTH_HOST", "")
	t.Setenv("FLOYD_OAUTH_HOST", "")

	_, err := buildAuthRuntime()
	require.ErrorIs(t, err, megafloydauth.ErrNotConfigured)
}
