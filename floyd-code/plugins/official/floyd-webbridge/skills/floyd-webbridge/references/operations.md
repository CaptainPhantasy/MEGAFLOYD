# Operations: daemon lifecycle and recovery

Read this only when a tool call can't reach the daemon, or the user explicitly asks to install / start / troubleshoot floyd-webbridge.

## The daemon

The `floyd-webbridge` binary lives at `~/.floyd-webbridge/bin/floyd-webbridge` (Windows: `%USERPROFILE%\.floyd-webbridge\bin\floyd-webbridge.exe`) and serves a local HTTP daemon on `127.0.0.1:10086`. Status, PID, and logs live under `~/.floyd-webbridge/`.

## Recovery — what to do when a tool call fails

1. **Daemon not reachable (connection refused)** → start it yourself, don't ask the user. `start` is idempotent: it no-ops if the daemon is already up, and concurrent starts converge to a single daemon (the OS lets only one process bind port 10086).
   - macOS / Linux: `~/.floyd-webbridge/bin/floyd-webbridge start`
   - Windows: `& "$env:USERPROFILE\.floyd-webbridge\bin\floyd-webbridge.exe" start`

   Then retry the tool call.
2. **`command not found` / binary missing** → not installed. Tell the user to reinstall the plugin from Floyd Code's `/plugins` panel, which downloads the daemon binary and the plugin together.
3. **Extension missing or won't connect** → give the user both installation paths:
   - Chrome Web Store: https://chromewebstore.google.com/detail/floyd-webbridge/fldmhceldgbpfpkbgopacenieobmligc
   - Restricted-network fallback: with the extension package already on disk, unzip it, open `chrome://extensions`, enable Developer mode, choose **Load unpacked**, and select the extracted folder.
4. **Anything still broken after a `start` + retry** → don't deep-troubleshoot. Point the user to the installation paths above, and suggest they report the problem at https://github.com/CaptainPhantasy/f7/issues with the daemon log from `~/.floyd-webbridge/`.

## Do NOT do automatically

Never run `stop` / `restart` / `uninstall` on your own. They kill the running daemon; if the user runs the **Floyd Desktop App** (which manages its own daemon), an external stop/restart also fights the app. If a hard restart is genuinely needed, ask the user to do it themselves — reopen the Floyd Desktop App, or run `floyd-webbridge restart` by hand.

## /status JSON fields

- `running` (bool) — daemon listening on `:10086`
- `version` (string) — daemon build version
- `extension_connected` (bool) — a WebSocket client (the browser extension) is attached
- `extension_id` (string) — the Chrome/Edge extension ID, empty if none
- `uptime_seconds` (int)
