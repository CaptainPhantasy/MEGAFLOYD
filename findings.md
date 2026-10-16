# findings.md — MEGAFLOYD reference (for the next model)

> Author note: curated so a fresh session can resume without re-discovery.
> Sources: `docs/MERGE-STATUS-20261016.md`, `docs/prompt-pack/MANIFEST.md`, `PLAN.md`,
> and independent verification run 2026-10-16.

## A. What MEGAFLOYD is

Work-in-progress merger of two Floyd lines into one build, at `/Users/floyd/MEGAFLOYD`
(`origin=github.com/CaptainPhantasy/MEGAFLOYD`), commit `e33af3f`. **Not wired to any
existing install.** Scratch project.

| Path | What | Origin |
|---|---|---|
| `floyd-code/` | Floyd Code v2.0.2, TS/pnpm monorepo | copy of `floyd-v6/KimiSource/kimi-code` (`.git`/`node_modules`/dists excluded) |
| `floyd-go/` | Go agent behaviour layer | copy of `floyd-v6/src` |
| `docs/prompt-pack/` | Go prompt templates + assembly ref, as plain data | `floyd-go/internal/agent/{templates,prompt}` |
| `PLAN.md` | phases P0–P5, plug points, gates, risks | pre-copy |
| `docs/MERGE-STATUS-20261016.md` | point-in-time audit of P0/P1 | this tree |
| `docs/prompt-pack/MANIFEST.md` | canonical pack interface + checksums | this tree |
| `scripts/verify-prompt-pack.mjs` | drift guard | this tree |

## B. Verified state (2026-10-16, re-checked)

| Claim | Result |
|---|---|
| `floyd-code/` = copy of kimi-code, exclusions only | TRUE |
| Floyd Code version 2.0.2 | TRUE (`apps/floyd-code/package.json`) |
| `floyd-go/` = copy of `floyd-v6/src`, one file differs | TRUE (`internal/event/event.go`) |
| `docs/prompt-pack/` byte-identical to source | TRUE |
| `scripts/verify-prompt-pack.mjs` | **PASS** (derived copy + unversioned copy both match canonical) |
| P0 build proof reproducible | **Improved** — `floyd-code/node_modules` and `apps/floyd-code/dist` are now PRESENT (MERGE-STATUS F5 said absent); still unverified end-to-end this session |

### Toolchain (measured 2026-10-16)

| Tool | Local | Required |
|---|---|---|
| node | v26.9.0 | `>=24.15.0` ✓ |
| pnpm | **10.25.0** | `pnpm@10.33.0` (packageManager) — mismatch, corepack fetches 10.33.0 |
| go | **go1.25.7** | `go.mod` says `go 1.26.6` → GOTOOLCHAIN download on first `go build` |

## C. The prompt-pack truth (F3, resolved)

- **Canonical:** `docs/prompt-pack/templates/` (8 files). Edit here only.
- **Derived (physical copy required by `//go:embed`):** `floyd-go/internal/agent/templates/`.
  Never edit directly. Sync with `node scripts/verify-prompt-pack.mjs --sync`.
- **Advisory (outside git):** `../floyd-merge/prompt-pack/templates/` — matches; not authoritative.
- The Go assembler's real interface is the `PromptDat` struct (`prompt.go:31`), **not** the
  variable names listed in `PLAN.md` P1. See `MANIFEST.md` §2.
- Only 3 templates (`coder`, `task`, `initialize`) are overridable from
  `<global config dir>/prompts/<name>.md.tpl`; the other 5 are embedded-only.

## D. Plug-point paths (F2, corrected)

| Task | Actual file(s) |
|---|---|
| Prompt/profile injection | `packages/agent-core-v2/src/app/agentProfileCatalog/{agentProfileCatalog.ts,profile-shared.ts}` |
| System/context files | `packages/agent-core-v2/src/workspace/workspaceAgentProfileLoader/internal/systemFile.ts`, `.../workspaceAgentProfileLoader/userAgentProfileLoaderService.ts` |
| Workspace instructions | `packages/agent-core-v2/src/workspace/workspaceInstructions/workspaceInstructionsService.ts`; `.../agent/agentsMdReminder/agentsMdReminderService.ts` |
| Session bootstrap | `packages/agent-core-v2/src/features/sessionInit/` |
| Tool discipline rules | `packages/agent-core-v2/src/features/externalHooks/agent/agentExternalHooksService.ts`; `.../externalHooks/internal/types.ts` |
| Compaction/summary | `packages/agent-core-v2/src/agent/fullCompaction/fullCompactionService.ts` (+ `compactionInstruction.ts`) |

All 11 paths verified present 2026-10-16.

## E. Secret / telemetry (F1)

- Merged `floyd-go/internal/event/event.go` reads `MEGAFLOYD_POSTHOG_KEY` from env; no key baked in. ✓
- `.gitleaks.toml` allowlists 4 reviewed paths only.
- **Open gap:** the hardcoded PostHog key `phc_4zt4…I0V` still lives in the SOURCE repo
  `floyd-v6/src/internal/event/event.go:18` (outside this project). Not fixable from here without
  touching a source repo. Recorded, not repaired.

## F. Housekeeping (F6)

- `floyd-go/.envrc`, `floyd-go/.github/`, `.gitattributes` copied in from source — dead config.
- `floyd-code/services/company-platform/` (Python + JS + MJS) — needs an explicit in/out decision
  before P5's egress review.

## G. Do-not-cache

- Tool counts, `reachable` flags, build-output presence (`node_modules`, `dist`) — all change
  between sessions. Re-measure.

## H. Session-2 defect inventory — "required artifact invisible to git" (VERIFIED)

Class: an over-broad, **unanchored** `.gitignore` rule (a bare directory/file name) matches a
real source path at any depth, so the file exists locally but is never committed — and is then
absent from every clone. One copy-time exclusion is in the same class. Six instances total.

| # | Artifact | Rule that hid it | State |
|---|---|---|---|
| 1 | `floyd-code/build/{raw-text-plugin,raw-text-loader,register-raw-text-loader}.mjs` | root `.gitignore` `build/` | fixed `9088cd4` |
| 2 | `apps/vis/web/src/components/logs/LogsTab.tsx` | root `.gitignore` `logs/` | fixed `f8c4541` |
| 3 | `packages/agent-core-v2/src/human/{test/,}credentials/*.ts` (5) | root `.gitignore` `credentials` | fixed `f8c4541` |
| 4 | `packages/migration-legacy/test/fixtures/{golden,multi-workdir}/.floyd/**` (26) | root `.gitignore` `.floyd/` | fixed `f8c4541` |
| 5 | `packages/klient/Dockerfile` | `floyd-code/.gitignore:26` bare `Dockerfile` | **open** |
| 6 | `apps/floyd-code/dist-web/**` (524) | copy exclusion (`.git/…/dist-native/dist-web` dropped) + root `dist-web/` | **open** |

Fix applied (`f8c4541`): a trailing **negation block** in the root `.gitignore` — explicit `!`
re-includes placed **after** the excluding patterns (last-match-wins). Negative controls confirm
`node_modules/`, `dist/`, `.floyd/`, `build/` at foreign depths stay ignored.

Sweep proof (VERIFIED 2026-10-16):
`comm -23 <(git -C <source> ls-files | sort) <(git -C <mega> ls-files -- floyd-code | sed 's#^floyd-code/##' | sort)`
→ **525 paths = 524 × dist-web + 1 × klient/Dockerfile**. No other omissions.

`dist-web` is a **committed, prebuilt** bundle (developed in the upstream `code-app` repo,
shipped here per `floyd-code/AGENTS.md` as "gitignored, force-added"). `check-web-assets.mjs`
makes packaging fail loudly if it is absent — correct behavior; the tree simply lost the file.

## I. Web console + plugin/marketplace surface

### I.1 The console is live (VERIFIED 2026-10-16)

- `https://<console-host>:8446/` serves the **Floyd Code Web** SPA
  (`<title>Floyd Code Web</title>`; served by the inference host over the tailnet; exact hostname
  held locally in `.floyd/.supercache`, not committed).
  Nav: *FloydCode · Providers · My account · Administration* — this is the admin console the
  owner referred to. `GET /healthz` → `{"ok": true}`.
- `/approve?user_code=<code>` is the **OAuth device-authorization approval page**
  (RFC 8628). The SPA's device-flow fields: `verification_uri_complete`, `user_code`,
  `expires_in`, `interval` → the CLI prints a `verification_uri_complete` of this shape.
- `GET /meta`, `GET /plugins/marketplace`, `GET /api/v1/*` → **404** unauthenticated.
  The API paths below are the ones the client requests; they are auth-gated (404 masks them),
  so they are NOT reachable without a session/token.

### I.2 Client API surface (VERIFIED — extracted from the committed `dist-web` bundle)

Path literals the web client requests (grouped; base is server-relative):

- auth: `/auth`, `/login`, `/oauth/login`, `/oauth/logout`, `/oauth/region`, `/oauth/usage`,
  `/oauth/userinfo`
- plugins / marketplace: `/plugins`, `/plugins/marketplace` (GET → `{entries}`),
  `/plugins/{id}:enable` `:disable` `:remove` (POST); event `event.plugin.changed` →
  `pluginsChanged`, `event.capability.changed`
- providers: `/providers`, `/providers:import_catalog`, `/providers:import_registry`,
  `/providers:refresh`, `/providers:refresh_oauth`, `/catalog/providers`
- sessions: `/sessions`, `/sessions/`, `/sessions:archive`, `/sessions:restore`, `/admin/sessions`
- workspaces/fs: `/workspaces`, `/workspace/fs:search`, `/workspace/fs:suggest`, `/files`,
  `/fs:browse`, `/fs:content`, `/fs:home`
- misc: `/capabilities`, `/config`, `/models`, `/healthz`, `/meta`, `/status`, `/devices/`,
  `/v1/remote/devices`, `/api/v1`, `/api/v2`
- client-side slash commands (not HTTP): `/btw` `/clear` `/compact` `/export` `/fork` `/goal`
  `/new` `/plan` `/swarm` `/tower` `/undo`

Track C0 note: the **plugin dashboard + marketplace are real and already wired** in the web
client — C2 is integration/reuse, not greenfield.

### I.3 Still UNVERIFIED (leads only)

- Owner note: the marketplace **CDN is "maybe" on Cloudflare** — no evidence yet. Confirm the
  marketplace catalog fetch origin before designing C2 distribution.
- "indeeence hosts the admin console" — the console is on the inference host (I.1); "indeeence"
  is unexplained. Do not treat as a fact.

## J. Toolchain reality check (2026-10-16)

- Mac (client-only): node v26.9.0 both trees; **pnpm 10.33.0** on the SanDisk1Tb clone.
- Inference host: Ubuntu 7.0.0-38, reached over the tailnet; exact address + login held locally
  in `.floyd/.supercache` (not committed). **no docker/podman** — plain node processes + systemd.
  MCP secret-broker runs there (port 9717).

## §K — Company platform build-out (Track E, in-VERIFIED 2026-10-16)

Owner reversed D-1: service is IN the ship build. Work landed this session:

- **Bootstrap** `491e1d7`: `load_config()` generated config.json + a one-time
  `ADMIN-PASSWORD.txt` (0600) when absent; previously the module crashed on import from a
  clean checkout. Guarded `98db924` (clean-dir import test + restart-reuse assertion).
- **Endpoint de-hardcode** `be0e4f0`: removed the internal console host from `company-tools.mjs`,
  the private IP from the provider seed, and the internal CDN from the console install link
  (now server-injected `window.__FLOYD_CONFIG`, `install_url`). gitleaks + grep clean.
- **Host-path de-hardcode** `86b7fb6`: removed every baked absolute host path
  (speech model under another user's home, `/srv/.../transcribe-cli`, `/usr/bin/ffmpeg`,
  `/opt/ComfyUI/temp`, linuxbrew node, llama-swap settings, MCP gateway root). Each now
  resolves config.json -> env -> PATH, degrading to a clear 'not configured' error.

**Egress inventory (read, not yet a runtime capture).** Outbound surface, by path:
- `company_platform.read_json_url` → `catalog_url` (default `https://models.dev/api.json`) and
  each configured provider `base_url` (`/models`). NoRedirect opener + 15 s timeout.
- `model_access.relay` → configured provider `base_url` (chat/completions, 120 s timeout).
- `shared_tools.bridge` → local child process (`gateway_bridge.mjs` via stdio MCP); no socket.
- `media_tools` → loopback `music_backend_url` (default 127.0.0.1:8188) + local `ffmpeg`/transcriber.
- `company-tools.mjs` (client-side) → member's `FLOYD_CODE_BASE_URL`.
No endpoint is reachable beyond configuration. **Still to do (E3):** capture this as a live
allow-list assertion rather than prose, then add the service to the P5 manifest (E5).
