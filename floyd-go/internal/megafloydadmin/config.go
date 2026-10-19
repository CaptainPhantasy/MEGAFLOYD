package megafloydadmin

import (
	"os"
	"strings"
	"time"
)

const (
	// DefaultUsersPath is the verified account-list route.
	DefaultUsersPath = "/admin/api/users"
	// DefaultInvitesPath is the verified invitation route.
	DefaultInvitesPath = "/admin/api/invites"
	// DefaultHTTPTimeout bounds one admin request.
	DefaultHTTPTimeout = 30 * time.Second
	// DefaultUserAgent identifies this client to the admin plane.
	DefaultUserAgent = "megafloyd-admin/1.0"
)

// EnvLookup matches os.Getenv. Tests pass a map-backed lookup.
type EnvLookup func(string) string

// Config is the resolved admin-plane configuration for one process.
type Config struct {
	// BaseURL is the admin origin, for example
	// https://admin.example.test.
	BaseURL string
	// UsersPath is the account-list route. Empty means DefaultUsersPath.
	UsersPath string
	// InvitesPath is the invitation route. Empty means DefaultInvitesPath.
	InvitesPath string
	// GrantPath is the per-user grant route. Empty means the route is
	// unverified and Grant fails closed.
	GrantPath string
	// RevokePath is the per-user revoke route. Empty means the route is
	// unverified and RevokeGrant fails closed.
	RevokePath string
	// GrantMethod is the HTTP method for GrantPath. Empty means POST.
	GrantMethod string
	// RevokeMethod is the HTTP method for RevokePath. Empty means POST.
	RevokeMethod string
	// UserAgent is sent on every request.
	UserAgent string
	// Timeout bounds one request. Zero means DefaultHTTPTimeout.
	Timeout time.Duration
	// HTTPClient is injectable for tests. Nil means a client with a
	// timeout that refuses redirects.
	HTTPClient HTTPDoer
	// Tokens supplies the access token. Required.
	Tokens TokenSource
}

// FromEnv resolves a Config.
//
// MEGAFLOYD_* names win over the FLOYD_CODE_* mirrors that the f7
// launcher exports, so an existing f7 environment works unchanged.
func FromEnv(lookup EnvLookup) Config {
	if lookup == nil {
		lookup = os.Getenv
	}
	return Config{
		BaseURL: firstNonEmpty(
			lookup("MEGAFLOYD_ADMIN_BASE_URL"),
			lookup("MEGAFLOYD_AUTH_BASE_URL"),
			lookup("FLOYD_CODE_BASE_URL"),
			lookup("MEGAFLOYD_AUTH_HOST"),
			lookup("FLOYD_CODE_OAUTH_HOST"),
			lookup("FLOYD_OAUTH_HOST"),
		),
		UsersPath: firstNonEmpty(
			lookup("MEGAFLOYD_ADMIN_USERS_PATH"),
			DefaultUsersPath,
		),
		InvitesPath: firstNonEmpty(
			lookup("MEGAFLOYD_ADMIN_INVITES_PATH"),
			DefaultInvitesPath,
		),
		GrantPath:    strings.TrimSpace(lookup("MEGAFLOYD_ADMIN_GRANT_PATH")),
		RevokePath:   strings.TrimSpace(lookup("MEGAFLOYD_ADMIN_REVOKE_PATH")),
		GrantMethod:  normalizeMethod(lookup("MEGAFLOYD_ADMIN_GRANT_METHOD")),
		RevokeMethod: normalizeMethod(lookup("MEGAFLOYD_ADMIN_REVOKE_METHOD")),
		UserAgent: firstNonEmpty(
			lookup("MEGAFLOYD_ADMIN_USER_AGENT"),
			DefaultUserAgent,
		),
		Timeout: parseTimeout(lookup("MEGAFLOYD_ADMIN_TIMEOUT")),
	}
}

func parseTimeout(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultHTTPTimeout
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return DefaultHTTPTimeout
	}
	return timeout
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
