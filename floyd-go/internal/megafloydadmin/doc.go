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
// Authentication is an Authorization: Bearer header carrying the access
// token from internal/megafloydauth. The live admin app itself signs in
// with an administrator key, keeps a same-origin session cookie, and sends
// X-CSRF-Token on writes; whether the server accepts a Bearer token on the
// admin routes is UNKNOWN. Token material is never included in errors or
// output.
package megafloydadmin
