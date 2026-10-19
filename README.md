# MEGAFLOYD Desktop

The desktop surface of MEGAFLOYD. A native macOS shell (Electron 43) forked from the
Kimi 3.2.17 desktop app, stripped of Kimi's cloud, updater, telemetry and product UI,
then pointed at the MEGAFLOYD platform: the Go chassis (`megafloyd`) and the Floyd Code
web engine it hands off to.

The desktop does not reimplement the engine. It launches the platform's own handoff —
`megafloyd --web` — with the platform's documented environment, reads back the tokenised
loopback URL, and supervises the result. One contract (`internal/megafloydweb`) serves
the CLI, the cloud and this app.

## What ships

```
MEGAFLOYD.app/
  Contents/
    MacOS/MEGAFLOYD                     Electron launcher (renamed, ad-hoc signed)
    Frameworks/MEGAFLOYD Helper*.app    Renamed Chromium helpers
    Resources/
      app.asar                          desktop/src (main, preload, status page)
      home.html                         status surface (outside asar by design)
      provenance.json                   base app, Electron, platform commit
      icon.icns                         generated MEGAFLOYD mark
      megafloyd/
        bin/megafloyd                   131.8 MB Go chassis
        bin/node                        ELECTRON_RUN_AS_NODE wrapper (no system Node)
        web/dist/main.mjs               Floyd Code 2.0.2 server
        web/dist-web/                   web UI bundle
        web/native/                     native assets
        web/node_modules/               production dependencies (pnpm deploy)
```

Home directory: `~/.megafloyd` (override with `MEGAFLOYD_HOME`).
Platform data: `~/.megafloyd/web` (sessions, workspaces, `server.token` at mode 0600).
Engine logs: `~/.megafloyd/logs/web-server.log`; desktop events: `~/.megafloyd/logs/desktop.log`.

## Build

Requires macOS (arm64), Node >= 22, and the pinned base app in `vendor/`.

```sh
make icon      # regenerate desktop/assets/icon.icns
make build     # vendor base -> build/MEGAFLOYD.app -> MEGAFLOYD.app
make verify    # 14 structural checks
make smoke     # headless: platform handoff, ready state as JSON
make smoke-ui  # window + UI load, prints title, bridge and URL, exits
make run       # launch the built app
```

Platform prerequisites (one-time, not part of `make build`):

```sh
cd platform-src/floyd-go   && GOTOOLCHAIN=auto go build -o ../bin/megafloyd .
cd platform-src/floyd-code && npx --yes pnpm@10.33.0 install --frozen-lockfile
npx --yes pnpm@10.33.0 run build:packages
npx --yes pnpm@10.33.0 --filter @legacy-ai/floyd-code run build
npx --yes pnpm@10.33.0 --filter @legacy-ai/floyd-code deploy --prod --legacy ../../platform/web
```

Pinned at `platform-src/PLATFORM-COMMIT` (`1d15945`). `platform/` and `platform-src/`
are build inputs and are not committed.

## Desktop surfaces

What the shell adds that a browser tab cannot:

| Surface | Behaviour |
|---|---|
| Global shortcut | `Cmd+Shift+Space` shows/hides the app from any application |
| Deep links | `megafloyd://session/<id>` (engine route), `megafloyd://workspace?path=<abs>` (restart engine there), anything else → status page |
| Open with / drag to Dock | `open-file` sets the engine workspace to the file's directory |
| Tray | Engine state, restart, show, quit |
| Menus | New window, open workspace, copy loopback URL, open in browser, restart, logs, home |
| Bridge | `info`, `getState`, `onState`, `restart`, `openApp`, `openLogs`, `openExternal`, `copy`, `chooseWorkspace`, `setBadge`, `notify` |

`setBadge` and `notify` are additive for the web UI; the current platform UI does not
call them yet (it uses the renderer `Notification` API, which the shell permits natively).

## Engine resolution

First match wins (spec C1):

1. `MEGAFLOYD_BIN`
2. `MEGAFLOYD.app/Contents/Resources/megafloyd/bin/megafloyd` (shipped)
3. `~/.megafloyd/bin/megafloyd`
4. `PATH`

Invocation: `megafloyd --web` with `MEGAFLOYD_HOME`, `MEGAFLOYD_WEB_PORT`,
`MEGAFLOYD_WEB_SCRIPT`, `MEGAFLOYD_WEB_BUNDLE`, `MEGAFLOYD_WEB_NODE`,
`FLOYD_DISABLE_TELEMETRY=1`. The handoff prints `Web UI: http://127.0.0.1:<port>/…`.

## Runtime guarantees

- **No Kimi network calls.** Gateway bundle, updater feed, telemetry resources and
  localization packs are deleted at build time. `app-update.yml` does not exist.
- **No Kimi identity.** Bundle id `ai.legacy.megafloyd`, name `MEGAFLOYD`, scheme
  `megafloyd`, app state under `~/.megafloyd/desktop`.
- **No remote surface.** Engine binds loopback only; the window navigates only to the
  engine origin or the local status page. New windows and off-origin links go to the
  system browser.
- **Renderer isolation.** `contextIsolation`, `sandbox`, no `nodeIntegration`, no
  `<webview>`, explicit permission allowlist.
- **Crash containment.** Handoff exit is tolerated (the server outlives it); health probe
  every 10 s, three failures trigger recovery, at most three restarts in five minutes.
- **Clean shutdown.** SIGTERM to the listener, 8 s grace, SIGKILL; no orphan process.
- **One instance.** Second launches focus the existing window.

## Status

Working and verified on this machine: build, 14/14 structural checks, headless handoff,
window + platform UI load, PTY spawn from the packaged tree, clean shutdown, no legacy
`~/.config/floyd` writes with a fresh `MEGAFLOYD_HOME`.

Not yet:

- Developer ID signing and notarization — ad-hoc only (owner deferred).
- Auto-update — deliberately removed; needs a MEGAFLOYD-owned feed.
- Windows/Linux packaging — the base app is arm64 macOS.
- Session store reconciliation between the Go chassis and the web engine (spec C6).
- Capability handshake against `/api/v1/meta` (spec C4) and a diagnostics bundle (C16).
- The web UI does not yet consume the desktop badge/notification calls.

## Provenance

Base: Kimi desktop 3.2.17 (`3.2.17.4`, Electron 43.7.3), extracted from
`kimi_3.2.17.dmg` (sha256 `9da2a4fc…`); the DMG ships a wrapper, the app is its
`Contents/Helpers/Kimi.app`. Platform: commit `1d15945`, Floyd Code 2.0.2.
The same data is written to `Contents/Resources/provenance.json` at build time.
See `docs/REBRAND.md` for the mutation list and `specs.md` for the contract.
