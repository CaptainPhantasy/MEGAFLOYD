// Package megafloydadmin is an HTTP client for the company admin plane
// served by the floyd-auth server.
//
// Verified 2026-10-19 from the live server's /platform.js (the admin
// single-page app) and unauthenticated GET probes:
//
//   - GET  /admin/api/users
//   - POST /admin/api/invites            body {"username": "..."} or {"user_id": "..."}
//   - POST /admin/api/users/{id}/enable
//   - POST /admin/api/users/{id}/disable
//   - GET  /admin/api/devices?status=pending
//   - POST /admin/api/devices/{device_code}/approve   body {"user_id": "..."}
//   - POST /admin/api/devices/{device_code}/deny
//   - GET/POST /admin/api/tools
//   - GET  /admin/api/tools/search?query=...
//   - GET/POST/PUT /admin/api/connections[/{id}]
//   - GET  /admin/api/overview
//
// The admin app also calls login, logout, key rotation, catalog refresh,
// and feedback routes. Those are outside this client's command surface.
//
// No per-user tool or model grant route appears anywhere in the admin app.
// Grant and RevokeGrant therefore fail closed with ErrUnverifiedEndpoint
// unless the operator supplies MEGAFLOYD_ADMIN_GRANT_PATH or
// MEGAFLOYD_ADMIN_REVOKE_PATH. This package never guesses a route.
//
// Two authentication modes are supported:
//
//   - Session mode mirrors the admin single-page app. It posts the
//     administrator key to POST /admin/api/login as {"key": "..."},
//     retains the session cookie from Set-Cookie, reads the CSRF token
//     from GET /admin/api/overview, and sends X-CSRF-Token on writes. Set
//     MEGAFLOYD_ADMIN_AUTH_MODE=session and MEGAFLOYD_ADMIN_KEY (or
//     MEGAFLOYD_ADMIN_PASSWORD), or just set the key: an unset mode
//     selects session mode when a key is present.
//   - Bearer mode, the fallback, sends Authorization: Bearer with the
//     access token from internal/megafloydauth. It is selected when
//     MEGAFLOYD_ADMIN_AUTH_MODE=bearer, or when no administrator key is
//     configured.
//
// The login and overview paths can be overridden with
// MEGAFLOYD_ADMIN_LOGIN_PATH and MEGAFLOYD_ADMIN_OVERVIEW_PATH. Key and
// token material never appear in errors or output.
package megafloydadmin
