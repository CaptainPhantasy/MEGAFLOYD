// Package megafloydauth implements the floyd-auth identity layer for
// megafloyd: the RFC 8628 device-code login, token persistence, and lazy
// refresh used against the owner's inference server.
//
// The wire shape (three POST endpoints, form encoding, snake_case token
// bundle) is taken from the donor f7 client in packages/oauth/src/oauth.ts
// and is documented in docs/auth-integration.md. The /api/oauth/* routes
// have not been called live from this package; every test uses httptest.
// Treat the live flow as unverified until it is exercised against the
// server.
//
// Tokens live under the megafloyd home (MEGAFLOYD_HOME, default
// ~/.megafloyd), never under f7's ~/.floyd7, so two harnesses cannot share
// one refresh token. Files are 0600 inside a 0700 directory and are written
// atomically.
//
// Manager.Token and Manager.AccessToken are the helper for future provider
// wiring. Nothing in the chassis consumes them yet.
package megafloydauth
