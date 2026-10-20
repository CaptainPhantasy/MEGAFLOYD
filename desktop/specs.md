# MEGAFLOYD Desktop — Platform Contract Specification

Status: owner-provided contract; statuses below reflect the build of 2026-10-19.
Scope: the agreements between the desktop shell (`desktop/`, Electron 43,
bundle `ai.legacy.megafloyd`) and the rest of the MEGAFLOYD platform: the Go
binary (`platform-src/floyd-go`), the Node engine and web UI
(`platform-src/floyd-code`, Floyd Code 2.0.2), and the owner's external
services (auth server, MCP room).

Pinned platform: `platform-src/PLATFORM-COMMIT` = `bd08789` (commit
`bd08789417b4db307962800b74457bb4c54cd4d5`).
App version: 1.0.0. Engine version: 2.0.2. Go version: stamped at build.

Rules for this document:
- MUST / SHOULD / MAY follow RFC 2119.
- Every item cites its current evidence or is marked OPEN.
- Changing a MUST requires bumping `PLATFORM-COMMIT` and updating this file.
- The desktop MUST tolerate unknown fields in platform responses
  (forward compatibility); the platform MUST NOT remove a field the desktop
  reads without a version bump.

## C1. Engine binary discovery

- **MUST**: the desktop discover the platform binary in this order:
  `MEGAFLOYD_BIN`, bundled `Contents/Resources/megafloyd/bin/megafloyd`,
  `$MEGAFLOYD_HOME/bin/megafloyd`, then `PATH`.
- **MUST**: report which source won in the status page and logs.
- Evidence: `desktop/src/config.mjs:33-45`.
- Status: MET.
- Acceptance: remove each source in turn; the next one is used and named.

## C2. Engine handoff invocation

- **MUST**: the desktop start the engine by calling the platform's own
  handoff (`megafloyd --web --no-open --port <port>`), not by reimplementing
  server startup.
- **MUST**: the handoff print a ready line containing a URL of the form
  `http://127.0.0.1:<port>/<path>#token=<token>` on stdout.
- **MUST**: exit nonzero with a diagnostic on stderr when the web bundle or
  Node runtime is missing.
- Evidence: `desktop/src/engine.mjs:19`, `:30-36`; `config.mjs:28-31`.
- NOTE: the Go handoff takes `--web` only; the port comes from `MEGAFLOYD_WEB_PORT`
  (`internal/megafloydweb/home.go:39`). `--no-open`/`--port` are flags of the Node
  `floyd web` command (`internal/megafloydweb/command.go:26`), not of `megafloyd --web`.
  This MUST is satisfied through the env form.
- Status: MET for invocation and URL shape; OPEN for a machine-readable
  ready line (a `--json` mode is not specified yet).
- Verified: `--smoke` on 2026-10-19 returned `source: bundled` and
  `http://127.0.0.1:58627/sessions/#token=…`.
- Acceptance: run the exact argv; parse the URL; confirm the token matches
  `$MEGAFLOYD_HOME/server.token` and the port is loopback.

## C3. Token and readiness contract

- **MUST**: the token live in `$MEGAFLOYD_HOME/server.token` (mode 0600) and
  reach the renderer only through the URL fragment.
- **MUST**: readiness be confirmable independently of the printed line via
  `GET /api/v1/meta` (token-authenticated where required).
- **MUST NOT**: the desktop persist the token in `localStorage`, logs, crash
  reports, or the status page.
- NOTE (platform build `bd08789`): the engine keeps the token in memory and
  prints it in the ready line; a fresh home contains no `server.token` while
  the server runs. The desktop consumes the printed URL and does not depend on
  the file. The platform-side token-file MUST remains unmet.
- Evidence: `desktop/src/engine.mjs` (bufferLines), `desktop/src/redact.mjs`,
  `desktop/test/redact.test.mjs`, `desktop/scripts/check-log-secrets.mjs`.
- Verified 2026-10-19: after `make smoke`, `desktop.log` contains only
  `#token=<redacted>` and `Web UI: <non-loopback URL suppressed>`; the scan
  reports 3/3. Renderer state is sanitized before the bridge, so diagnostics
  copy cannot carry the token.
- Status: MET for the desktop; the token-file half is OPEN on the platform.
- Acceptance: grep the app logs and state dir for the token string; zero
  hits.

## C4. Version and capability handshake

- **MUST**: `/api/v1/meta` report at least: engine version, platform commit,
  protocol version, and a capability list (web UI, sessions API, plan mode,
  jobs, terminal, MCP, auth provider).
- **MUST**: the desktop display engine version + platform commit in the
  status page and refuse to load a UI whose protocol major differs.
- **SHOULD**: the desktop warn (not fail) on a newer engine with unknown
  capabilities.
- Evidence: `desktop/src/meta.mjs`; `desktop/src/engine.mjs` (`becomeReady`);
  `desktop/test/meta.test.mjs`. The descriptor is fetched with the fragment
  token before any UI load and recorded in engine state.
- Verified 2026-10-19: `make smoke` reports `engineVersion: "2.0.2"`,
  `protocolMajor: 2`, all six capabilities, no warnings; `make smoke-ui`
  still loads the platform UI. A meta fixture with major 3 is refused.
- NOTE: the pinned platform meta does not expose `protocol_version` or
  `platform_commit`. The desktop falls back to the server-version major and
  shows the build provenance commit as `(bundled)`; a declared
  `protocol_version` wins when present.
- Status: PARTIAL — desktop handshake and refusal are MET; the platform fields
  the MUST names are not reported yet.
- Acceptance: meta fixture with old/new major versions; correct refuse/warn
  behavior in tests.

## C5. Home, config, data, and logs

- **MUST**: one home for desktop, CLI, and engine:
  `MEGAFLOYD_HOME` or `~/.megafloyd`.
- **MUST**: `MEGAFLOYD_*` values override legacy `FLOYD_*` mirrors; legacy
  paths MUST NOT be written when a MEGAFLOYD home is set.
- **MUST**: the engine receive the same home so config, skills, commands,
  jobs, plans, and the session store agree with the CLI.
- Evidence: `config.mjs:8-22`; platform hardening landed in commit `bd08789`.
- Verified 2026-10-19: with a fresh `MEGAFLOYD_HOME`, a full run created nothing
  under `~/.config/floyd` or `~/.local/share/floyd` (mtime check).
- Status: PARTIAL. Known leaks found in the platform audit:
  `~/.local/share/floyd/{providers.json,hyper.json}` (provider cache),
  `~/.config/floyd/skills`, `~/.config/floyd/commands`.
- Acceptance: launch with a fresh `MEGAFLOYD_HOME`; `find ~/.config ~/.local`
  shows no new Floyd paths.

## C6. Session model and store

- **MUST**: define one authoritative session store for the platform, or a
  documented mirror. Today the Go chassis stores sessions in `<data>/floyd.db`
  with UUID ids, while the Node engine stores `session_<uuid>` under
  `FLOYD_CODE_HOME`; neither sees the other.
- **MUST**: a desktop deep link or resume action MUST NOT silently open an
  empty transcript for a session that exists in the other store.
- **SHOULD**: the desktop show which engine owns the session and offer the
  matching surface.
- Evidence: `desktop/src/session-store.mjs`;
  `desktop/src/main.mjs` (`routeSession`); `desktop/test/session-store.test.mjs`.
  A deep link is resolved against both stores before navigation; `session_<uuid>`
  and `<uuid>` are tried as mirrors. Chassis-only, not-found, and unverifiable
  sessions open the status page with a notice naming the store and a
  `megafloyd -s <id>` resume hint.
- Verified 2026-10-19 against the live engine: an engine session resolved to
  `owner: engine` / load UI; a row in a real `floyd.db` resolved to
  `owner: chassis` / `session-store-mismatch`; an id in neither resolved to
  `owner: none` / `session-not-found`. A bare UUID matched `session_<uuid>`,
  and a prefixed id matched the chassis UUID.
- CLI side (2026-10-20): `docs/session-bridge.md` adds
  `megafloyd session engines` and `megafloyd session import <id>`. Imports
  keep the engine ID and are idempotent; verified live with real engine
  sessions of 1,019 and 5,694 messages, and `session show` renders the
  imported transcript.
- Export side (2026-10-20): `megafloyd session export <id> --engine-home
  <dir>` writes `state.json`, `session_index.jsonl` and an engine-shaped
  `wire.jsonl`. Verified live: exported a 5,694-message CLI session into a
  scratch engine home, started the engine against it, and the history route
  returned 50 turns with 50 user and 46 assistant messages, assistant text
  present. The engine therefore reads CLI sessions through the bridge.
- Status: PARTIAL — desktop resolution, the CLI import bridge and the
  engine-side export path are MET; live sync between the two stores and a
  single authoritative store remain OPEN.
- Acceptance: create a session in each surface; list both; resume each from
  the desktop; assert the transcript is present or an explicit error names
  the store mismatch.

## C7. Authentication and credentials

- **MUST**: company login use the owner's auth server device flow
  (`/api/oauth/device_authorization`, `/api/oauth/token`, client id pinned by
  the platform); tokens stored 0600 under `$MEGAFLOYD_HOME/credentials`.
- **MUST**: the desktop never request, store, display, or rotate the admin
  key. Admin functions stay in the browser admin surface.
- **MUST**: provider credentials reach the engine only through the platform
  config; the desktop MUST NOT hold API keys.
- Evidence: platform `internal/megafloydauth`, `docs/auth-integration.md`;
  desktop preload has no credential surface (`preload.cjs`).
- Status: PARTIAL (auth server flow implemented and verified at the HTTP
  level; live token refresh and provider use are not exercised end-to-end).
- Acceptance: log in from the UI; model call succeeds; token file mode 0600;
  no key material in desktop state.

## C8. Models and provider catalog

- **MUST**: the engine select models/providers; the desktop does not
  duplicate selection logic.
- **MUST**: provider auto-update stay disabled in offline/air-gapped use, and
  a cached catalog be authoritative when the network fails.
- **SHOULD**: menu items that open model/provider surfaces deep-link into
  the engine UI rather than maintaining duplicate lists.
- Evidence: platform provider cache paths; desktop menus (`desktop/src/menu.mjs`).
- Status: PARTIAL.
- Acceptance: launch with the network down; the last catalog loads; model
  switch works inside the web UI.

## C9. MCP room and external services

- **MUST**: MCP configuration belong to the engine/CLI config; the desktop
  never speaks to the MCP room directly.
- **MUST**: the room URL be configurable by env/config with a documented
  precedence (home tailnet URL vs cloud public URL).
- **MUST NOT**: the desktop open sockets to any origin other than the engine
  loopback origin.
- Evidence: `docs/ARCHITECTURE.md` "No remote surface"; `preload.cjs` has no
  network surface.
- Status: MET by design; room URL contract belongs to platform config.

## C10. Commands, skills, and UI parity

- **MUST**: every command the desktop UI advertises resolve in the engine;
  no dead palette entries.
- **MUST**: plan mode, jobs tools, and the command registry behave the same
  as the CLI's documented surface, or the desktop hide what the engine does
  not support.
- **MUST**: skills installed into `<data>/commands` be visible in the desktop
  UI after an engine restart.
- Evidence: platform command registry and skills installer; desktop renders
  the engine UI unchanged.
- Status: PARTIAL (parity depends on engine UI, not the shell).
- Acceptance: a parity test enumerates the platform command list and asserts
  each is present or explicitly marked unsupported in the web UI.

## C11. Terminal panes

- **MUST**: either terminal panes work in the packaged engine, or the entry
  point is removed/disabled with a user-facing reason.
- **MUST**: node-pty prebuilds be verified in the deployed tree (the build
  skips install scripts).
- Evidence: `desktop/scripts/build.mjs` (`fixSpawnHelpers`);
  `docs/VERIFICATION.md` "PTY from the packaged tree".
- Verified 2026-10-19: `node-pty` loads and spawns from the packaged tree once
  `spawn-helper` is executable (pnpm skipped the install script, leaving 0644).
- Status: PARTIAL — the PTY primitive works; a terminal pane inside the UI has
  not been exercised.
- Acceptance: open a terminal pane in the packaged app; run a command; close
  cleanly; no orphan processes.

## C12. Deep links

- **MUST**: define the `megafloyd://` route table, minimum:
  `megafloyd://session/<id>`, `megafloyd://workspace?path=<abs>`,
  `megafloyd://settings`.
- **MUST**: unknown routes open the app home, never an error dialog.
- **MUST**: session routes resolve against the authoritative store (C6).
- Evidence: `desktop/src/main.mjs` `routeURL`, `routeSession`, `routeWorkspace`.
  Session links queue until the engine is ready; unknown routes open the status page.
- Status: PARTIAL — implemented and registered; LaunchServices delivery not yet
  exercised in a GUI session.
- Acceptance: open each route on a cold app; correct surface; unknown route
  falls back home.

## C13. Renderer IPC bridge

- **MUST**: the preload expose exactly these calls and keep their names:
  `info`, `getState`, `onState`, `restart`, `openApp`, `openLogs`,
  `openExternal`, `copy`, `chooseWorkspace`.
- **MUST**: every main-process handler validate the sender is an app window.
- **MUST**: adding a call be additive; removing or renaming one requires an
  app version bump.
- Evidence: `desktop/src/preload.cjs`; `docs/ARCHITECTURE.md` "Renderer
  boundary".
- Additive since the draft: `setBadge`, `notify` (sender-validated; the current
  platform UI does not call them yet).
- Status: MET.
- Acceptance: bridge snapshot test; sender-spoof test rejects.

## C14. Security boundary

- **MUST**: engine binds loopback only; `--host` is never passed.
- **MUST**: `contextIsolation` + `sandbox` on; `nodeIntegration` and
  `webviewTag` off; explicit permission allowlist; off-origin navigation and
  new windows go to the system browser.
- **MUST NOT**: the desktop implement remote/Tailscale serving; that stays an
  engine feature (`--web` handoff).
- NOTE: the engine may advertise a non-loopback (Tailscale) `Web UI:` URL in
  the handoff output. That is an engine feature; the desktop never navigates to
  it and suppresses the URL in `desktop.log` (`redact.mjs`).
- Evidence: `docs/ARCHITECTURE.md`; README "Runtime guarantees";
  `desktop/test/redact.test.mjs`.
- Status: MET.
- Acceptance: attempt off-origin navigation and `window.open`; system
  browser opens, app window unchanged.

## C15. Lifecycle and failure containment

- **MUST**: spawn → parse ready → load UI; health probe every 10 s with 3
  failures triggering recovery; at most 3 auto-restarts per 5 minutes, then
  the status page with log path.
- **MUST**: shutdown via SIGTERM, 8 s grace, SIGKILL; no orphan engine.
- **MUST**: second launch focuses the existing window.
- Evidence: `docs/ARCHITECTURE.md` "Engine lifecycle"; `engine.mjs` constants.
- Status: MET.
- Acceptance: kill the engine mid-session; observe restart and UI recovery;
  quit; assert no listener remains on the port.

## C16. Logs and diagnostics

- **MUST**: engine stdout/stderr and desktop events land in
  `$MEGAFLOYD_HOME/logs`, with the exact paths shown in the status page.
- **SHOULD**: a one-click diagnostics bundle (versions, platform commit,
  health snapshot, log tail) with token and credential redaction.
- Evidence: `config.mjs:17-18`; status page fields in `home.html`.
- Status: PARTIAL (bundle undefined).
- Acceptance: produce the bundle; grep for the token and provider keys; zero
  hits.

## C17. Updates and release coupling

- **MUST**: the desktop ship a pinned engine and platform commit; app updates
  replace both together.
- **MUST NOT**: reintroduce the Kimi update feed or any vendor endpoint.
- **MUST**: define a MEGAFLOYD-owned update channel before auto-update
  returns (signed manifest, rollback, version floor). Until then updates are
  manual and the app must not check for updates.
- Evidence: README "Not yet"; `docs/REBRAND.md`.
- Status: OPEN (by design).
- Acceptance: app performs zero update-network calls (packet/log evidence).

## C18. Packaging, signing, provenance

- **MUST**: Developer ID signing + notarization before distribution beyond
  this machine; current signature is ad-hoc.
- **MUST**: bundle identity `ai.legacy.megafloyd`, product name `MEGAFLOYD`,
  helper names rebranded, deep-link scheme `megafloyd`.
- **MUST**: record base app provenance (Kimi 3.2.17, Electron 43.7.3) and the
  platform commit inside the artifact.
- Evidence: `build.mjs` writes `Contents/Resources/provenance.json`; `verify.mjs`
  fails the gate when it is missing or incomplete (2026-10-19: Kimi 3.2.17 /
  Electron 43.7.3 / platform `bd08789`).
- Status: PARTIAL — provenance is recorded and gated; Developer ID signing and
  notarization are deferred by the owner.
- Acceptance: `codesign --verify --deep --strict` and `spctl` pass on a
  distribution build; provenance file present in Resources.

## C19. Telemetry and egress

- **MUST**: no third-party telemetry in the desktop or engine by default;
  egress limited to the configured model/provider endpoints, the auth server,
  and the MCP room (all engine-side).
- **MUST**: document every allowed destination per release.
- Evidence: platform telemetry opt-in (`MEGAFLOYD_POSTHOG_KEY`); desktop has
  no network surface beyond the engine.
- Status: MET/PARTIAL (no explicit per-release destination list yet).
- Acceptance: trace app startup and one model call; destinations match the
  documented allowlist.

## C20. Verification gates

- **MUST**: `make verify` cover identity, asar contents, engine presence,
  signature, and Electron fuses; `make smoke` prove headless engine boot;
  `make smoke-ui` prove window load.
- **MUST**: the platform CI (linux build, installer, cloud) and the desktop
  gates both pass on the pinned platform commit before a release.
- **SHOULD**: contract tests for C2, C3, C4, C13, and C15 run in the desktop
  repo without a GUI.
- Evidence: `desktop/scripts/verify.mjs` (14 checks, 14/14 on 2026-10-19);
  `--smoke` and `--smoke-ui` both pass against the platform handoff.
- Status: PARTIAL (desktop gates exist and pass; no contract tests yet; desktop
  gates not wired into platform CI).
- Acceptance: CI run on the pinned commit produces both artifact sets and
  both gate reports.

## Contract change log

| Date | Change | Platform commit |
|---|---|---|
| 2026-10-19 | Initial draft from the desktop shell as built | `bd08789` |

Open items, in contract order: C6 (unified store), C16 (bundle), C17 (update
channel), C18 (signing), C20 cross-CI. Partials: C4 (platform fields), C5, C6
(desktop resolution), C7, C8, C10, C11, C12, C19.
