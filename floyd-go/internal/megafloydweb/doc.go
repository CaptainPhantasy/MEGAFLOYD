// Package megafloydweb hands the running megafloyd process over to the Node
// web server that serves the prebuilt Floyd Code web client.
//
// The Go binary is the engine and the TUI. The web client is a prebuilt
// bundle served by the Node server in floyd-code, so web mode means: detect
// the Tailscale identity, prepare a child environment the server accepts,
// spawn the server detached, and print a ready-to-click URL.
//
// Verified behaviour this package relies on, from
// floyd-code/apps/floyd-code/src/cli/sub/web and
// floyd-code/packages/kap-server/src:
//
//   - `floyd web` takes --no-open, --port, --host [host], --allowed-host,
//     --insecure-no-tls, and --rc.
//   - The server writes its bearer token to <FLOYD_CODE_HOME>/server.token.
//   - FLOYD_CODE_ALLOWED_HOSTS is read from the environment and merged with
//     --allowed-host. Raw IP addresses are always allowed; DNS names are
//     not, so the MagicDNS name must be in the allowlist.
//   - Remote control publishes <FLOYD_CODE_HOME>/server/rc.json while it
//     runs and requires FLOYD_CODE_REMOTE_CONTROL_RELAY_URL.
//
// Nothing here starts a real server in tests.
package megafloydweb
