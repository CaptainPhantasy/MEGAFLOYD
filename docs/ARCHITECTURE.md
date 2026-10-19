# Architecture

## Processes

```
MEGAFLOYD.app/Contents/MacOS/MEGAFLOYD          main process (Electron)
├── renderer (Chromium)                          Floyd Code web UI, sandboxed
└── handoff child                                Resources/megafloyd/bin/megafloyd --web
    ├── node                                     ELECTRON_RUN_AS_NODE wrapper
    └── Floyd Code server 2.0.2                  HTTP + WebSocket on 127.0.0.1
```

The app never starts the server itself. It calls the platform's documented handoff and
reads back what the handoff prints, so the CLI, the cloud and the desktop share one code
path (`internal/megafloydweb`). The Node runtime is the app's own Electron binary
re-entered with `ELECTRON_RUN_AS_NODE=1`; no system Node install is required.

The handoff process exits once the server is up and the URL is printed. That exit is
expected: the listener is a detached child, and the desktop supervises it by port.

## Why the Kimi gateway was removed

Kimi's agent runtime (`openclaw`, 189 MB) owns its own model credentials, workspace, and
WebSocket protocol. That is a second product with a second brain. MEGAFLOYD's brain is the
Floyd engine — the Go chassis and the Floyd Code server — so the gateway bundle and its
native modules are deleted. The shell keeps what is expensive and neutral: Electron,
Chromium, window management, menus, tray, notifications, file dialogs, deep links.

## Engine lifecycle

```
idle ──start──▶ starting ──ready signal──▶ ready
                    │                        │
                    ├─ timeout/exit ─▶ failed / crashed ──auto-restart (≤3 / 5 min)
                    │
                    └─ stop ─▶ stopped
```

- Discovery order is fixed in `desktop/src/config.mjs`.
- Port: prefers 58627, falls back to an ephemeral loopback port.
- Readiness: the handoff prints `Web UI: http://127.0.0.1:<port>/sessions/#token=<token>`;
  the main process parses that line. If it is missed, the token file is read and
  `/api/v1/meta` probed.
- Restart policy: 1.5 s × attempt, at most three restarts per five minutes, then the
  status page with the log path.
- Shutdown: SIGTERM, 8 s grace, SIGKILL. The engine is killed when the app quits, so no
  orphan server survives.

## Token handling

The engine writes `~/.megafloyd/web/server.token` (0600) on first boot. The main process
keeps the token in the URL fragment only — fragments are not sent to servers in HTTP
requests, so the token never appears in access logs or referrers. The window is bound to
the engine origin; navigation elsewhere opens the system browser.

## Renderer boundary

| Control | Value |
|---|---|
| `contextIsolation` | true |
| `sandbox` | true |
| `nodeIntegration` | false |
| `webviewTag` | false |
| preload | `src/preload.cjs`, exposes eleven bridge calls |
| new windows | denied; URL handed to `shell.openExternal` |
| off-origin navigation | denied; URL handed to `shell.openExternal` |
| permissions | clipboard, notifications, media, fullscreen only |

Every IPC handler re-checks that the sender is one of the app's own windows.

## What the status page is for

Startup is not instant (engine boot ≈ 1 s on this machine, longer on cold caches). The
window opens immediately on `desktop/src/home.html` and swaps to the UI when the engine
signals ready. The same page is the failure surface: status, engine path, port, error,
restart, logs. It is served from `file://` and cannot reach the network.

## Desktop surfaces

Native behaviour that the platform UI cannot reach from a browser:

- `Cmd+Shift+Space` toggles the window (`globalShortcut`, registration verified).
- `megafloyd://session/<id>`, `megafloyd://workspace?path=<abs>`; unknown routes fall
  back to the status page. A session link that arrives before the engine is ready is
  queued, not dropped.
- `open-file` (Finder "Open With", drag to Dock) points the engine at the file's directory.
- `setBadge` and `notify` bridge calls, sender-validated, for UI-initiated signals.

## Deliberate omissions

- No preload access to the filesystem. File operations belong to the engine and the web UI.
- No auto-update. The Kimi feed is gone; a MEGAFLOYD feed does not exist yet.
- No remote bind. `--host` is never passed; Tailscale handoff stays an engine feature.
