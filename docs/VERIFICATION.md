# Verification — 2026-10-19 (platform build)

Machine: macOS (arm64), Electron 43.7.3, Node 26.9.0.
Artifact: `MEGAFLOYD.app` 1.0.0, built from `vendor/Kimi-3.2.17-arm64.app` with the
platform at `platform-src/PLATFORM-COMMIT` = `1d15945417b4db307962800b74457bb4c54cd4d5`.

## Build provenance

| Item | Value |
|---|---|
| Source DMG | `kimi_3.2.17.dmg` |
| DMG SHA-256 | `9da2a4fc17d1ddaeb5044e1fea4e89679d36a6a011b2f5c0d44593d516b24423` |
| Kimi version | 3.2.17 (build 3.2.17.4) |
| Electron | 43.7.3 |
| Platform binary | `megafloyd` (Go), 131.8 MB, `Resources/megafloyd/bin/megafloyd` |
| Web engine | Floyd Code 2.0.2, `pnpm deploy --prod --legacy`, 117 MB tree |
| Product | bundle id `ai.legacy.megafloyd`, scheme `megafloyd` |
| Provenance file | `Contents/Resources/provenance.json` (written by `build.mjs`) |
| Signature | ad-hoc (`codesign --deep --sign -`); notarization deferred |

## Structural gate (`make verify`)

```
PASS  app bundle exists
PASS  bundle identity  ai.legacy.megafloyd 1.0.0
PASS  executable present
PASS  helpers renamed  4 helpers
PASS  kimi cloud resources removed
PASS  app.asar contents  5 required entries
PASS  platform binary embedded  131.8 MB
PASS  platform web embedded  17.6 MB server + bundle
PASS  asar integrity fuse disabled  fuse off
PASS  status page installed  6928 bytes
PASS  icon installed  150625 bytes
PASS  provenance recorded  Kimi 3.2.17 / Electron 43.7.3 / 1d15945
PASS  code signature  ad-hoc signature valid
PASS  no quarantine attribute  clean

14/14 checks passed
```

`codesign` failed on the first platform build because pnpm's deploy tree carried a
dangling workspace self-link; `build.mjs` now prunes dangling symlinks before signing.

## Headless handoff (`--smoke`)

```
{"ok":true,"state":{"status":"ready","detail":"Engine ready on port 58627",
 "port":58627,"url":"http://127.0.0.1:58627/sessions/#token=…",
 "entry":"…/Resources/megafloyd/bin/megafloyd","source":"bundled",
 "webLog":"…/logs/web-server.log"}}
```

`source: "bundled"` proves discovery picked the embedded binary, not `PATH`.

## Log redaction and remote-URL suppression

```
$ make test
✔ 9/9 redaction and state-sanitisation tests pass

$ make smoke
...
node desktop/scripts/check-log-secrets.mjs /tmp/megafloyd-smoke
PASS  desktop log written  1 log file(s)
PASS  no token values in desktop log  token values redacted
PASS  no non-loopback Web UI URL in desktop log  remote handoff URLs suppressed

3/3 checks passed
```

`desktop.log` after the run carries `Local: .../#token=<redacted>` and
`Web UI: <non-loopback URL suppressed>`. The raw ready line is still parsed for
readiness; only the persisted copy is redacted. Renderer-bound state is sanitized before
the IPC bridge, so the diagnostics copy cannot carry the token either.

## Window + UI (`--smoke-ui`)

```
{"engine":{"ok":true,…"source":"bundled"},
 "ui":{"ok":true,"title":"Floyd Code","windowTitle":"MEGAFLOYD",
       "bridge":"object","url":"http://127.0.0.1:58627/sessions/"}}
```

`bridge: "object"` proves the sandboxed preload loaded from inside `app.asar`;
`windowTitle: "MEGAFLOYD"` proves native branding survives the UI's own document title.

## Capability handshake (`/api/v1/meta`, spec C4)

```
engineVersion: "2.0.2"
protocolMajor: 2
capabilities: { websocket, file_upload, fs_query, mcp, tasks, terminal }
metaWarnings: []
```

The descriptor is fetched with the token from the ready URL fragment before the window
navigates. Unit tests cover the pinned contract, a major-3 refusal (including a declared
`protocol_version`), unknown capabilities, missing advertised capabilities, failed
features, a 401, and a non-zero envelope. `platformCommit` is `null` because the pinned
engine does not report it; the status page shows the build provenance commit with a
`(bundled)` marker.

## Live launch and shutdown

- `MEGAFLOYD_HOME=/tmp/mf-live ./MEGAFLOYD.app/Contents/MacOS/MEGAFLOYD` → listener on
  `127.0.0.1:58627`; `GET /` → 200.
- `kill -TERM <main pid>` → process gone, port closed within 5 s.
- Home layout with a fresh home: `<home>/{web,logs,desktop}`; `web/{sessions,server,
  workspaces.json,device_id,server.token}`; `server.token` mode `0600`.
- No files were created under `~/.config/floyd` or `~/.local/share/floyd` during a run
  with a fresh `MEGAFLOYD_HOME` (checked by mtime).

## PTY from the packaged tree

```
$ Resources/megafloyd/bin/node -e "require('…/web/node_modules/node-pty').spawn('/bin/sh',['-c','echo PTY_FROM_BUNDLE'],…)"
PTY_FROM_BUNDLE
exit 0
```

Root cause of the earlier failure: pnpm skipped node-pty's install script, leaving
`prebuilds/darwin-arm64/spawn-helper` at mode 0644; every spawn failed with
`posix_spawnp failed`. `build.mjs` now chmods `spawn-helper` to 0755.

## Not verified

- Clicked interactions inside the web UI (sessions, model calls, terminals).
- Deep links delivered by Finder/LaunchServices (`megafloyd://…`) and the global
  shortcut; both are implemented and registered, but a GUI session is required to
  exercise them.
- Terminal pane inside the UI end to end (the PTY primitive itself is verified above).
- Session store reconciliation between the Go chassis and the web engine (spec C6).
- Developer ID signature and notarization (deferred by the owner).
