# progress.md — MEGAFLOYD completion session log

## 2026-10-16 — session 1

### Phase 1 — complete: recon

- Read `README.md`, `PLAN.md`, `docs/MERGE-STATUS-20261016.md`, `docs/prompt-pack/MANIFEST.md`.
- Git: single commit `e33af3f` (initial scaffold); untracked = `docs/MERGE-STATUS-20261016.md`,
  `docs/prompt-pack/MANIFEST.md`, `scripts/`. No `.planning/`.
- Ran `node scripts/verify-prompt-pack.mjs` → **PASS** (derived + unversioned copies match).
- Measured toolchain: node v26.9.0 ✓, pnpm 10.25.0 (pinned 10.33.0), go go1.25.7 (go.mod 1.26.6).
- `floyd-code/node_modules` and `apps/floyd-code/dist` are now present (they were absent when
  MERGE-STATUS F5 was written).
- Verified all 11 corrected plug-point paths exist under `floyd-code/packages/agent-core-v2/src`.
- Confirmed F1: source repo `floyd-v6/src` still carries the PostHog key; merged copy remediated.

### Planning files created (this repo, tracked)

- `task_plan.md`, `findings.md`, `progress.md` at repo root, so memory travels with the project.

### Files written this session

- `/Users/floyd/MEGAFLOYD/task_plan.md`
- `/Users/floyd/MEGAFLOYD/findings.md`
- `/Users/floyd/MEGAFLOYD/progress.md` (this file)

### Phase 2–7 — complete: closure tranche

- **P1 prompt pack** closed: canonical = `docs/prompt-pack/templates/`; derived Go copy documented;
  `MANIFEST.md` present with the real `PromptDat` interface; guard PASS.
- **`PLAN.md`** corrected: plug-point table rewritten to the 11 verified paths; P1 variable list
  replaced with a pointer to `MANIFEST.md` §2 (the old CWD/ENV/GIT_STATUS/… names do not exist);
  stale `/Users/floyd/floyd-merge/prompt-pack/` framing replaced with the in-repo canonical pack.
- **`README.md`**: added a Prerequisites table (node/pnpm/go measured vs required) + `GOTOOLCHAIN`
  note + drift-guard usage; Status section now names P0/P1 done and links the docs.
- **`docs/DECISIONS.md`** created: D-1 company-platform = OUT of the ship build (reasoned);
  D-2 source-repo PostHog key recorded (owner-owned); D-3 prompt-pack single-source-of-truth decided.

### Phase 8 — gates green, committed

| Gate | Command | Result |
|---|---|---|
| Drift guard | `node scripts/verify-prompt-pack.mjs` | PASS |
| Floyd Code | `node floyd-code/apps/floyd-code/dist/main.mjs --version` | 2.0.2 |
| Go build | `go build ./...` | exit 0 |
| Go test | `go test ./internal/shell/...` | ok 4.890s |
| Go vet | `go vet ./internal/agent/...` | clean |

P2–P5 remain unstarted (roadmap in `task_plan.md`), not claimed done.

## 2026-10-16 — session 2 (build relocation to /Volumes/SanDisk1Tb/MEGAFLOYD)

### Trigger

Owner relocated the build to a fresh clone and ran `pnpm -r run build`; it failed repeatedly on
files that exist in local copies but were never committed.

### Diagnosis — one defect class, six instances (see `findings.md` §H)

Over-broad unanchored `.gitignore` rules hide real source at any depth.

- Fixed root-file instances 1–4 with a trailing negation block (`f8c4541`): `build/`, `logs/`,
  `credentials`, `.floyd/`. Verified with `git check-ignore` + negative controls.
- Committed + pushed from `/Users/floyd/MEGAFLOYD` (fast-forwarded to `d62c595` first).
- Sweep proof: `comm` of source-vs-mega tracked files = 525 = 524 `dist-web` + 1 `klient/Dockerfile`.

### Then — third blocker, still the same class

`check-web-assets.mjs` aborts: committed `apps/floyd-code/dist-web/` missing. Open → Track A A3.

### Planning files

- Rewrote `task_plan.md` as the master plan (Tracks A–D, gates, operating rules).
- Extended `findings.md` (§H inventory, §I unverified infra leads, §J toolchain).
- Added `docs/DECISIONS.md` D-4 (ignore-rule policy + guard) and D-5 (dist-web restore policy).

## 2026-10-16 — session 3 (build proof reproduced; Track A closed)

### Gate — fresh-copy build proof: PASSED

| Step | Command | Result |
|---|---|---|
| Install | `pnpm install --frozen-lockfile` in `floyd-code/` | exit 0; corepack used the pinned pnpm 10.33.0 |
| Build | `pnpm -r run build` (root script `build`; matches the source README's `pnpm build`) | exit 0; 23 packages reported Done |
| App version | `node apps/floyd-code/dist/main.mjs --version` | `2.0.2`, exit 0 |
| Web assets | `node apps/floyd-code/scripts/check-web-assets.mjs` | "Web assets OK … 525 entries" |

Toolchain measured: node v26.9.0, pnpm 10.25.0 on PATH (pinned 10.33.0 resolved by corepack),
go 1.25.7 against a `go.mod` that declares 1.26.6.

### First attempt failed, and this is what it was

`pnpm -r run build` before installing failed with `sh: tsdown: command not found` for
`kaos`, `kosong`, `minidb`, `oauth`. Those workspaces call `tsdown` in their build script,
but declare it only at the root, and their per-package dependencies had never been
installed in this copy — so the tool was not resolvable. The documented install step fixed
it; no config was changed. After install the same build exits 0.

### Corrections to checks made earlier in this session

- **A2 is closed**: `floyd-code/packages/klient/Dockerfile` IS tracked and is not ignored.
  The earlier check used the wrong root path.
- **A3 is closed**: `apps/floyd-code/dist-web` is present with 525 tracked entries. The gate
  script lives at `floyd-code/apps/floyd-code/scripts/check-web-assets.mjs`, not `scripts/`.

### Added

- `FLOYD.md` at the project root: identity, operating rules, and the order of work, so a
  session opening in this folder does not need telling what the project is. This is also the
  file phase B3 will read.

- 2026-10-19 ~01:20: checkpoint written (process fix). Wave 3 still running. Web/backend correction recorded: exists, ships as-is, nothing rebuilt.
- 2026-10-19 ~02:30: OWNER-CONFIRMED GAP. Intelligence layer content was never ported: donor-only templates (floyd_protocol, floyd-general, protocol_kernel, superfloyd-coder, mcp_tools_reference, deterministic/) absent; 7 same-name templates differ; prompt assembly (prompt.go vs prompts.go) unreconciled; chassis runs stock Crush coder.md.tpl; prompt-pack is inert. Fix work dispatched (work/intelligence-prompt). Verification must be behavioral: assembled-prompt diff vs donor, not file presence.
- 2026-10-19 ~02:30: INTELLIGENCE LAYER PORTED AND MERGED (work/intelligence-prompt, c8550b5).
  Templates byte-identical to donor (8/8 shasum MATCH): protocol_kernel, floyd_protocol,
  floyd-general, superfloyd-coder, initialize, summary, title, agentic_fetch + deterministic/.
  Kernel injected as its own system message after prefix, before role. Dynamic context
  (cwd/git/date/.supercache) now built via BuildCacheablePrompts (was dead code) and injected
  uncached. PromptDat extended with AvailMCPXML/AvailPluginXML/PluginInstrXML (empty until
  chassis registry exposes XML helpers; runtime MCP instructions already injected separately).
  Gates: build 0; agent suite -race green; render + selection + kernel tests PASS.
  Binary rebuilt: dist/darwin-arm64/megafloyd, version c3d2f6f, smoke OK.
  UNVERIFIED: live provider session (no credentials exercised), real API-call kernel position.
