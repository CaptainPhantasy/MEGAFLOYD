package megafloydadmin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testToken = "test-access-token-do-not-print"

type staticTokenSource struct {
	token string
	err   error
}

func (s staticTokenSource) AccessToken(context.Context) (string, error) {
	return s.token, s.err
}

func newClientForServer(t *testing.T, server *httptest.Server, mutate func(*Config)) *Client {
	t.Helper()
	cfg := Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Tokens:     staticTokenSource{token: testToken},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	client, err := New(cfg)
	require.NoError(t, err)
	return client
}

func assertRequest(t *testing.T, r *http.Request, method, path string) {
	t.Helper()
	require.Equal(t, method, r.Method)
	require.Equal(t, path, r.URL.EscapedPath())
	require.Equal(t, "Bearer "+testToken, r.Header.Get("Authorization"))
	require.Equal(t, "application/json", r.Header.Get("Accept"))
	require.Equal(t, DefaultUserAgent, r.Header.Get("User-Agent"))
}

func TestListUsers(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodGet, "/admin/api/users")
		require.Empty(t, r.Header.Get("Content-Type"))
		_, _ = io.WriteString(w, `{"users":[
			{"id":"u1","username":"alice@example.test","enabled":true,
			 "last_login_at":"2026-10-01T12:00:00Z"},
			{"id":"u2","username":"bob","enabled":false,"last_login_at":null}
		]}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	users, err := client.ListUsers(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "u1", users[0].ID)
	require.Equal(t, "alice@example.test", users[0].Username)
	require.True(t, users[0].Enabled)
	require.Equal(t, "2026-10-01T12:00:00Z", users[0].LastLoginString())
	require.False(t, users[1].Enabled)
	require.Equal(t, "never", users[1].LastLoginString())
}

func TestInviteSendsVerifiedField(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/admin/api/invites")
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"username":"person@example.test"}`, string(body))
		_, _ = io.WriteString(w, `{"url":"https://example.test/join#token"}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	invite, err := client.Invite(context.Background(), "person@example.test")
	require.NoError(t, err)
	require.Equal(t, "https://example.test/join#token", invite.URL)
}

func TestUnauthorizedDoesNotLeakToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized","error_description":"Enter your administrator key."}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
	require.Contains(t, err.Error(), "Enter your administrator key.")
	require.NotContains(t, err.Error(), testToken)
}

func TestServerEchoedTokenIsRedacted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(
			w,
			`{"error":"server_error","error_description":"rejected `+testToken+`"}`,
		)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), testToken)
	require.Contains(t, err.Error(), "[redacted]")
}

func TestEndpointNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not_found"}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.ErrorIs(t, err, ErrEndpointNotFound)
}

func TestGrantFailsClosedWithoutVerifiedRoute(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.Grant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "tool",
		Name:  "search",
	})
	require.ErrorIs(t, err, ErrUnverifiedEndpoint)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_GRANT_PATH")
	require.Zero(t, calls.Load(), "unverified grant must not make a request")
}

func TestGrantUsesConfiguredRoute(t *testing.T) {
	t.Parallel()

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(
			t, r, http.MethodPut,
			"/admin/api/users/"+url.PathEscape("person@example.test")+"/grants",
		)
		var err error
		gotBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, func(cfg *Config) {
		cfg.GrantPath = "/admin/api/users/{email}/grants"
		cfg.GrantMethod = "PUT"
	})
	result, err := client.Grant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "tool",
		Name:  "search",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.JSONEq(t, `{"ok":true}`, string(result.Body))
	require.JSONEq(
		t,
		`{"email":"person@example.test","kind":"tool","name":"search"}`,
		string(gotBody),
	)
}

func TestRevokeUsesConfiguredRoute(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/admin/api/revoke")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newClientForServer(t, server, func(cfg *Config) {
		cfg.RevokePath = "/admin/api/revoke"
	})
	result, err := client.RevokeGrant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "model",
		Name:  "gpt-5",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, result.StatusCode)
	require.Empty(t, result.Body)
}

func TestFindUserAndDisable(t *testing.T) {
	t.Parallel()

	var disabled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/api/users":
			_, _ = io.WriteString(w, `{"users":[
				{"id":"u1","username":"Alice","enabled":true}
			]}`)
		case r.Method == http.MethodPost &&
			r.URL.Path == "/admin/api/users/u1/disable":
			disabled.Store(true)
			_, _ = io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	user, err := client.DisableUser(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "u1", user.ID)
	require.Equal(t, "Alice", user.Username)
	require.False(t, user.Enabled)
	require.True(t, disabled.Load())
}

func TestDisableAlreadyDisabledUserSkipsWrite(t *testing.T) {
	t.Parallel()

	var writes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes.Add(1)
		}
		_, _ = io.WriteString(w, `{"users":[
			{"id":"u1","username":"alice","enabled":false}
		]}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	user, err := client.DisableUser(context.Background(), "alice")
	require.NoError(t, err)
	require.False(t, user.Enabled)
	require.Zero(t, writes.Load())
}

func TestFindUserAmbiguous(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"users":[
			{"id":"u1","username":"Alice","enabled":true},
			{"id":"u2","username":"alice","enabled":true}
		]}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.FindUser(context.Background(), "ALICE")
	require.ErrorIs(t, err, ErrUserAmbiguous)
}

func TestFindUserMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"users":[]}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, nil)
	_, err := client.FindUser(context.Background(), "nobody")
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestRedirectIsNotFollowed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/api/users" {
			w.Header().Set("Location", "/admin/login")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		t.Errorf("redirect was followed to %s", r.URL.Path)
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Tokens:  staticTokenSource{token: testToken},
	})
	require.NoError(t, err)
	_, err = client.ListUsers(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 303")
	require.Contains(t, err.Error(), "/admin/login")
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Tokens: staticTokenSource{token: "x"}})
	require.ErrorIs(t, err, ErrNotConfigured)

	_, err = New(Config{BaseURL: "ftp://example.test", Tokens: staticTokenSource{token: "x"}})
	require.ErrorIs(t, err, ErrNotConfigured)

	_, err = New(Config{BaseURL: "https://example.test"})
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestFromEnvPrecedence(t *testing.T) {
	t.Setenv("MEGAFLOYD_ADMIN_BASE_URL", "https://admin.example.test")
	t.Setenv("MEGAFLOYD_AUTH_BASE_URL", "https://auth.example.test")
	t.Setenv("FLOYD_CODE_BASE_URL", "https://floyd.example.test")
	t.Setenv("MEGAFLOYD_ADMIN_USERS_PATH", "/custom/users")
	t.Setenv("MEGAFLOYD_ADMIN_INVITES_PATH", "")
	t.Setenv("MEGAFLOYD_ADMIN_GRANT_PATH", "  /custom/grant  ")
	t.Setenv("MEGAFLOYD_ADMIN_REVOKE_PATH", "")
	t.Setenv("MEGAFLOYD_ADMIN_GRANT_METHOD", "patch")
	t.Setenv("MEGAFLOYD_ADMIN_TIMEOUT", "12s")

	cfg := FromEnv(nil)
	require.Equal(t, "https://admin.example.test", cfg.BaseURL)
	require.Equal(t, "/custom/users", cfg.UsersPath)
	require.Equal(t, DefaultInvitesPath, cfg.InvitesPath)
	require.Equal(t, "/custom/grant", cfg.GrantPath)
	require.Empty(t, cfg.RevokePath)
	require.Equal(t, "PATCH", cfg.GrantMethod)
	require.Equal(t, 12*time.Second, cfg.Timeout)
}

func TestFromEnvFallsBackToAuthHost(t *testing.T) {
	t.Setenv("MEGAFLOYD_ADMIN_BASE_URL", "")
	t.Setenv("MEGAFLOYD_AUTH_BASE_URL", "")
	t.Setenv("FLOYD_CODE_BASE_URL", "")
	t.Setenv("MEGAFLOYD_AUTH_HOST", "https://auth.example.test")

	cfg := FromEnv(nil)
	require.Equal(t, "https://auth.example.test", cfg.BaseURL)
}

func TestUnverifiedErrorsDoNotLeakToken(t *testing.T) {
	t.Parallel()

	client, err := New(Config{
		BaseURL: "https://admin.example.test",
		Tokens:  staticTokenSource{token: testToken},
	})
	require.NoError(t, err)
	_, err = client.Grant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "tool",
		Name:  "search",
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrUnauthorized))
	require.NotContains(t, err.Error(), testToken)
	require.True(t, strings.Contains(err.Error(), "unverified endpoint"))
}

const (
	testAdminKey      = "fixture-admin-key-do-not-print"
	testLoginCSRF     = "login-csrf-fixture"
	testOverviewCSRF  = "overview-csrf-fixture"
	testSessionCookie = "fixture-session-cookie"
)

func newSessionClient(
	t *testing.T, server *httptest.Server, mutate func(*Config),
) *Client {
	t.Helper()
	cfg := Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		AuthMode:   AuthModeSession,
		AdminKey:   testAdminKey,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	client, err := New(cfg)
	require.NoError(t, err)
	return client
}

func TestSessionLoginOverviewCSRFAndCookieReuse(t *testing.T) {
	t.Parallel()

	var logins, overviews atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(
			t, r.Header.Get("Authorization"),
			"session mode must not send a Bearer header",
		)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/api/login":
			logins.Add(1)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"key":"`+testAdminKey+`"}`, string(body))
			http.SetCookie(w, &http.Cookie{
				Name:     "floyd_admin",
				Value:    testSessionCookie,
				Path:     "/",
				HttpOnly: true,
			})
			_, _ = io.WriteString(w, `{"csrf":"`+testLoginCSRF+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/api/overview":
			overviews.Add(1)
			require.Equal(
				t, "floyd_admin="+testSessionCookie, r.Header.Get("Cookie"),
			)
			_, _ = io.WriteString(w,
				`{"csrf":"`+testOverviewCSRF+`","accounts":{"enabled":1}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/admin/api/invites":
			require.Equal(
				t, "floyd_admin="+testSessionCookie, r.Header.Get("Cookie"),
			)
			require.Equal(t, testOverviewCSRF, r.Header.Get("X-CSRF-Token"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(
				t, `{"username":"person@example.test"}`, string(body),
			)
			_, _ = io.WriteString(
				w, `{"url":"https://example.test/join#token"}`,
			)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/api/users":
			require.Equal(
				t, "floyd_admin="+testSessionCookie, r.Header.Get("Cookie"),
			)
			_, _ = io.WriteString(w, `{"users":[
				{"id":"u1","username":"alice@example.test","enabled":true}
			]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	invite, err := client.Invite(context.Background(), "person@example.test")
	require.NoError(t, err)
	require.Equal(t, "https://example.test/join#token", invite.URL)

	users, err := client.ListUsers(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "alice@example.test", users[0].Username)

	require.EqualValues(t, 1, logins.Load(), "login must happen once")
	require.EqualValues(t, 1, overviews.Load(), "overview must happen once")
}

func TestSessionUsesOverviewCSRFOverLoginCSRF(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/login":
			http.SetCookie(w, &http.Cookie{
				Name: "floyd_admin", Value: testSessionCookie, Path: "/",
			})
			_, _ = io.WriteString(w, `{"csrf":"`+testLoginCSRF+`"}`)
		case "/admin/api/overview":
			_, _ = io.WriteString(w, `{"csrf":"`+testOverviewCSRF+`"}`)
		case "/admin/api/invites":
			require.Equal(t, testOverviewCSRF, r.Header.Get("X-CSRF-Token"))
			_, _ = io.WriteString(w, `{"url":"https://example.test/join"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.Invite(context.Background(), "person@example.test")
	require.NoError(t, err)
}

func TestSessionLoginRejectsKeyWithoutLeakingIt(t *testing.T) {
	t.Parallel()

	var overviews atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/api/overview" {
			overviews.Add(1)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(
			w,
			`{"error":"unauthorized","error_description":"rejected `+
				testAdminKey+`"}`,
		)
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
	require.NotContains(t, err.Error(), testAdminKey)
	require.Contains(t, err.Error(), "[redacted]")
	require.Zero(t, overviews.Load(), "a failed login must not load overview")
}

func TestSessionLoginRequiresSessionCookie(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"csrf":"`+testLoginCSRF+`"}`)
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.ErrorIs(t, err, ErrInvalidResponse)
	require.Contains(t, err.Error(), "no session cookie")
}

func TestSessionOverviewFailureFailsClosed(t *testing.T) {
	t.Parallel()

	var users atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/login":
			http.SetCookie(w, &http.Cookie{
				Name: "floyd_admin", Value: testSessionCookie, Path: "/",
			})
			_, _ = io.WriteString(w, `{"csrf":"`+testLoginCSRF+`"}`)
		case "/admin/api/overview":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not_found"}`)
		default:
			users.Add(1)
			_, _ = io.WriteString(w, `{"users":[]}`)
		}
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.ListUsers(context.Background())
	require.ErrorIs(t, err, ErrEndpointNotFound)
	require.Zero(t, users.Load(), "the overview failure must stop the request")
}

func TestSessionGrantAndRevokeRemainFailClosed(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.Grant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "tool",
		Name:  "search",
	})
	require.ErrorIs(t, err, ErrUnverifiedEndpoint)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_GRANT_PATH")

	_, err = client.RevokeGrant(context.Background(), GrantRequest{
		Email: "person@example.test",
		Kind:  "model",
		Name:  "gpt-5",
	})
	require.ErrorIs(t, err, ErrUnverifiedEndpoint)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_REVOKE_PATH")
	require.Zero(t, calls.Load(), "fail-closed grants must not log in")
}

func TestSessionRenewsExpiredReadOnce(t *testing.T) {
	t.Parallel()

	var logins, overviews, users atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/login":
			attempt := logins.Add(1)
			http.SetCookie(w, &http.Cookie{
				Name:  "floyd_admin",
				Value: "session-" + strconv.FormatInt(attempt, 10),
				Path:  "/",
			})
			_, _ = io.WriteString(w, `{"csrf":"login-csrf"}`)
		case "/admin/api/overview":
			attempt := overviews.Add(1)
			_, _ = io.WriteString(
				w,
				`{"csrf":"overview-csrf-`+strconv.FormatInt(attempt, 10)+`"}`,
			)
		case "/admin/api/users":
			users.Add(1)
			if r.Header.Get("Cookie") == "floyd_admin=session-1" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
				return
			}
			_, _ = io.WriteString(w, `{"users":[
				{"id":"u1","username":"alice","enabled":true}
			]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	usersResult, err := client.ListUsers(context.Background())
	require.NoError(t, err)
	require.Len(t, usersResult, 1)
	require.EqualValues(t, 2, logins.Load())
	require.EqualValues(t, 2, overviews.Load())
	require.EqualValues(t, 2, users.Load())
}

func TestSessionDoesNotRetryRejectedWrite(t *testing.T) {
	t.Parallel()

	var logins, invites atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/api/login":
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{
				Name: "floyd_admin", Value: testSessionCookie, Path: "/",
			})
			_, _ = io.WriteString(w, `{"csrf":"`+testLoginCSRF+`"}`)
		case "/admin/api/overview":
			_, _ = io.WriteString(w, `{"csrf":"`+testOverviewCSRF+`"}`)
		case "/admin/api/invites":
			invites.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newSessionClient(t, server, nil)
	_, err := client.Invite(context.Background(), "person@example.test")
	require.ErrorIs(t, err, ErrUnauthorized)
	require.EqualValues(t, 1, logins.Load())
	require.EqualValues(t, 1, invites.Load(), "a write must never be retried")
}

func TestSessionModeRequiresAdminKey(t *testing.T) {
	t.Parallel()

	_, err := New(Config{
		BaseURL:  "https://admin.example.test",
		AuthMode: AuthModeSession,
	})
	require.ErrorIs(t, err, ErrNotConfigured)
	require.Contains(t, err.Error(), "MEGAFLOYD_ADMIN_KEY")
	require.NotContains(t, err.Error(), testAdminKey)
}

func TestBearerModeStaysAvailableWhenKeyIsSet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodGet, "/admin/api/users")
		_, _ = io.WriteString(w, `{"users":[]}`)
	}))
	defer server.Close()

	client := newClientForServer(t, server, func(cfg *Config) {
		cfg.AuthMode = AuthModeBearer
		cfg.AdminKey = testAdminKey
	})
	users, err := client.ListUsers(context.Background())
	require.NoError(t, err)
	require.Empty(t, users)
}

func TestResolvedAuthModeAndSessionEnv(t *testing.T) {
	t.Setenv("MEGAFLOYD_ADMIN_AUTH_MODE", "admin")
	t.Setenv("MEGAFLOYD_ADMIN_KEY", "env-admin-key")
	t.Setenv("MEGAFLOYD_ADMIN_PASSWORD", "")
	t.Setenv("MEGAFLOYD_ADMIN_LOGIN_PATH", "/custom/login")
	t.Setenv("MEGAFLOYD_ADMIN_OVERVIEW_PATH", "/custom/overview")

	cfg := FromEnv(nil)
	mode, err := cfg.ResolvedAuthMode()
	require.NoError(t, err)
	require.Equal(t, AuthModeSession, mode)
	require.Equal(t, "env-admin-key", cfg.AdminKey)
	require.Equal(t, "/custom/login", cfg.LoginPath)
	require.Equal(t, "/custom/overview", cfg.OverviewPath)

	t.Setenv("MEGAFLOYD_ADMIN_AUTH_MODE", "")
	t.Setenv("MEGAFLOYD_ADMIN_KEY", "")
	t.Setenv("MEGAFLOYD_ADMIN_PASSWORD", "env-password")
	cfg = FromEnv(nil)
	mode, err = cfg.ResolvedAuthMode()
	require.NoError(t, err)
	require.Equal(t, AuthModeSession, mode)
	require.Equal(t, "env-password", cfg.AdminKey)

	t.Setenv("MEGAFLOYD_ADMIN_PASSWORD", "")
	cfg = FromEnv(nil)
	mode, err = cfg.ResolvedAuthMode()
	require.NoError(t, err)
	require.Equal(t, AuthModeBearer, mode)

	cfg = Config{AdminKey: "only-key"}
	mode, err = cfg.ResolvedAuthMode()
	require.NoError(t, err)
	require.Equal(t, AuthModeSession, mode)

	cfg = Config{AuthMode: "token"}
	mode, err = cfg.ResolvedAuthMode()
	require.NoError(t, err)
	require.Equal(t, AuthModeBearer, mode)

	_, err = Config{AuthMode: "bogus"}.ResolvedAuthMode()
	require.ErrorIs(t, err, ErrNotConfigured)
}
