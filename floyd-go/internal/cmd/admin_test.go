package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/legacy-ai/floyd/internal/megafloydadmin"
	"github.com/legacy-ai/floyd/internal/megafloydauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAdminTokens struct {
	token string
	err   error
}

func (f fakeAdminTokens) AccessToken(context.Context) (string, error) {
	return f.token, f.err
}

func newAdminTestClient(
	t *testing.T, server *httptest.Server,
	mutate func(*megafloydadmin.Config),
) *megafloydadmin.Client {
	t.Helper()
	cfg := megafloydadmin.Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Tokens:     fakeAdminTokens{token: "test-token"},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	client, err := megafloydadmin.New(cfg)
	require.NoError(t, err)
	return client
}

func useAdminRuntime(t *testing.T, client *megafloydadmin.Client) {
	t.Helper()
	previous := adminRuntimeFactory
	adminRuntimeFactory = func() (*adminRuntime, error) {
		return &adminRuntime{Client: client}, nil
	}
	t.Cleanup(func() { adminRuntimeFactory = previous })
}

func executeAdmin(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := newAdminCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func TestAdminUsersList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/admin/api/users", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"users":[
			{"id":"u1","username":"alice@example.test","enabled":true,
			 "last_login_at":"2026-10-01T12:00:00Z"},
			{"id":"u2","username":"bob","enabled":false,"last_login_at":null}
		]}`)
	}))
	defer server.Close()

	useAdminRuntime(t, newAdminTestClient(t, server, nil))
	output, err := executeAdmin(t, "users", "list")
	require.NoError(t, err)
	require.Contains(t, output, "USERNAME")
	require.Contains(t, output, "alice@example.test")
	require.Contains(t, output, "enabled")
	require.Contains(t, output, "2026-10-01T12:00:00Z")
	require.Contains(t, output, "bob")
	require.Contains(t, output, "disabled")
	require.Contains(t, output, "never")
}

func TestAdminInvite(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/admin/api/invites", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		gotBody = string(body)
		_, _ = io.WriteString(w, `{"url":"https://example.test/join#secret"}`)
	}))
	defer server.Close()

	useAdminRuntime(t, newAdminTestClient(t, server, nil))
	output, err := executeAdmin(t, "invite", "person@example.test")
	require.NoError(t, err)
	require.JSONEq(t, `{"username":"person@example.test"}`, gotBody)
	require.Contains(t, output, "Invitation link: https://example.test/join#secret")
}

func TestAdminGrantFailsClosed(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	useAdminRuntime(t, newAdminTestClient(t, server, nil))
	_, err := executeAdmin(
		t, "grant", "alice@example.test", "tool", "search",
	)
	require.ErrorIs(t, err, megafloydadmin.ErrUnverifiedEndpoint)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_GRANT_PATH")
	require.Zero(t, calls.Load())
}

func TestAdminRevokeDisablesAccount(t *testing.T) {
	var disabled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/admin/api/users" {
			_, _ = io.WriteString(w, `{"users":[
				{"id":"u1","username":"alice@example.test","enabled":true}
			]}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/admin/api/users/u1/disable" {
			disabled.Store(true)
			_, _ = io.WriteString(w, `{}`)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	useAdminRuntime(t, newAdminTestClient(t, server, nil))
	output, err := executeAdmin(t, "revoke", "alice@example.test")
	require.NoError(t, err)
	require.True(t, disabled.Load())
	require.Contains(t, output, "Disabled account alice@example.test (u1).")
}

func TestAdminRevokeGrantFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unverified revoke must not make a request")
	}))
	defer server.Close()

	useAdminRuntime(t, newAdminTestClient(t, server, nil))
	_, err := executeAdmin(t, "revoke", "alice@example.test", "model", "gpt-5")
	require.ErrorIs(t, err, megafloydadmin.ErrUnverifiedEndpoint)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_REVOKE_PATH")
}

func TestAdminHelpAndArgumentErrors(t *testing.T) {
	output, err := executeAdmin(t)
	require.NoError(t, err)
	require.Contains(t, output, "Manage company accounts and access")

	output, err = executeAdmin(t, "users")
	require.NoError(t, err)
	require.Contains(t, output, "List company accounts")

	_, err = executeAdmin(t, "invite")
	require.Error(t, err)

	_, err = executeAdmin(t, "grant", "alice@example.test", "tool")
	require.Error(t, err)
	require.Contains(t, err.Error(), "name is required")

	_, err = executeAdmin(t, "revoke")
	require.Error(t, err)
}

func TestAdminNotLoggedInHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request must not be sent without a token")
	}))
	defer server.Close()

	client, err := megafloydadmin.New(megafloydadmin.Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Tokens:     fakeAdminTokens{err: megafloydauth.ErrNotLoggedIn},
	})
	require.NoError(t, err)
	useAdminRuntime(t, client)

	_, err = executeAdmin(t, "users", "list")
	require.ErrorIs(t, err, megafloydauth.ErrNotLoggedIn)
	require.Contains(t, err.Error(), "auth login")
}

func TestParseAccessArgs(t *testing.T) {
	request, err := parseAccessArgs(
		[]string{"person@example.test", "tool:search"}, "",
	)
	require.NoError(t, err)
	require.Equal(t, "person@example.test", request.Email)
	require.Equal(t, "tool", request.Kind)
	require.Equal(t, "search", request.Name)

	request, err = parseAccessArgs(
		[]string{"person@example.test", "model", "gpt-5"}, "",
	)
	require.NoError(t, err)
	require.Equal(t, "model", request.Kind)
	require.Equal(t, "gpt-5", request.Name)

	request, err = parseAccessArgs(
		[]string{"person@example.test", "tool"}, "search",
	)
	require.NoError(t, err)
	require.Equal(t, "tool", request.Kind)
	require.Equal(t, "search", request.Name)

	_, err = parseAccessArgs(
		[]string{"person@example.test", "role", "admin"}, "",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tool or model")
}

func TestRootRegistersAdminCommand(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"admin", "users", "list"})
	require.NoError(t, err)
	require.Equal(t, "list", command.Name())
	require.Contains(t, command.CommandPath(), "admin users list")
}

func TestBuildAdminRuntimeSessionMode(t *testing.T) {
	const (
		fixtureKey    = "fixture-admin-key"
		fixtureCookie = "fixture-session"
		overviewCSRF  = "overview-csrf"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/login":
			require.Equal(t, http.MethodPost, r.Method)
			require.Empty(t, r.Header.Get("Authorization"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"key":"`+fixtureKey+`"}`, string(body))
			http.SetCookie(w, &http.Cookie{
				Name: "floyd_admin", Value: fixtureCookie, Path: "/",
			})
			_, _ = io.WriteString(w, `{"csrf":"login-csrf"}`)
		case "/admin/api/overview":
			require.Equal(t, "floyd_admin="+fixtureCookie, r.Header.Get("Cookie"))
			_, _ = io.WriteString(w, `{"csrf":"`+overviewCSRF+`"}`)
		case "/admin/api/users":
			require.Equal(t, "floyd_admin="+fixtureCookie, r.Header.Get("Cookie"))
			_, _ = io.WriteString(w, `{"users":[
				{"id":"u1","username":"alice@example.test","enabled":true}
			]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("MEGAFLOYD_ADMIN_BASE_URL", server.URL)
	t.Setenv("MEGAFLOYD_ADMIN_AUTH_MODE", "session")
	t.Setenv("MEGAFLOYD_ADMIN_KEY", fixtureKey)
	t.Setenv("MEGAFLOYD_ADMIN_PASSWORD", "")

	previousAuth := authRuntimeFactory
	authRuntimeFactory = func() (*authRuntime, error) {
		t.Error("session mode must not read the stored access token")
		return nil, megafloydauth.ErrNotLoggedIn
	}
	t.Cleanup(func() { authRuntimeFactory = previousAuth })

	previousAdmin := adminRuntimeFactory
	adminRuntimeFactory = buildAdminRuntime
	t.Cleanup(func() { adminRuntimeFactory = previousAdmin })

	output, err := executeAdmin(t, "users", "list")
	require.NoError(t, err)
	require.Contains(t, output, "alice@example.test")
}

func TestBuildAdminRuntimeDefaultsToBearer(t *testing.T) {
	for _, name := range []string{
		"MEGAFLOYD_ADMIN_BASE_URL",
		"MEGAFLOYD_AUTH_BASE_URL",
		"FLOYD_CODE_BASE_URL",
		"MEGAFLOYD_AUTH_HOST",
		"FLOYD_CODE_OAUTH_HOST",
		"FLOYD_OAUTH_HOST",
		"MEGAFLOYD_ADMIN_AUTH_MODE",
		"MEGAFLOYD_ADMIN_KEY",
		"MEGAFLOYD_ADMIN_PASSWORD",
	} {
		t.Setenv(name, "")
	}

	var called atomic.Bool
	previous := authRuntimeFactory
	authRuntimeFactory = func() (*authRuntime, error) {
		called.Store(true)
		return &authRuntime{
			Host:    "https://auth.example.test",
			Manager: &megafloydauth.Manager{},
		}, nil
	}
	t.Cleanup(func() { authRuntimeFactory = previous })

	runtime, err := buildAdminRuntime()
	require.NoError(t, err)
	require.NotNil(t, runtime.Client)
	require.True(t, called.Load(), "Bearer mode must use the token manager")
}
