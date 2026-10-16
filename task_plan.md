# task_plan.md — MEGAFLOYD: land the merge, then make it the tool

> Master plan. Supersedes the P0/P1-only framing. Kept in git so it travels with the project.

## Owner objective (verbatim, 2026-10-16)

- "complete MEGAFLOYD"
- "make the megafloyd package the ultimate tool for me but keep your core workethic in tact,
  its the best part of all of it and the part that was missing from the rest and what I came
  back for."
- standing: "ALSO FIND A WAY TO HOLD ON TO MEMORY. you keep losing it for some reason."
- standing: "vet everything before writing it as truth."

## Operating rules (the workethic — non-negotiable)

1. **Verify before asserting.** No claim of state without the command + its output.
2. **Evidence tags on every fact:** `VERIFIED` (ran it), `HYPOTHESIS` (reasoned, unrun),
   `LEAD` (heard/heuristically inferred — never promoted without a run).
3. **Memory persists in two places, both in git or the standard cache:**
   `task_plan.md` / `findings.md` / `progress.md` + `docs/DECISIONS.md` (tracked) and
   `.floyd/.supercache` (local cache). Checkpoint at every task boundary and mode switch.
4. **One hypothesis, one fix, observe.** On no change: invalidate, retract, do not loop.
5. **Gates are the only proof of "done".** A phase closes with a recorded gate result, never
   with prose.
6. **Never touch a source repo** (`floyd-v6/src`, `floyd-v6/KimiSource/kimi-code`). Read only.

## Where we are (2026-10-16, session 2)

- P0/P1 closed and committed at `4e85ae0`; build **relocated** to a fresh clone
  `/Volumes/SanDisk1Tb/MEGAFLOYD`.
- The fresh clone's `pnpm -r run build` failed **three times on the same defect class**:
  a required artifact exists in the working copy but is invisible to git, so it is absent
  from a clone. Root cause is a set of **over-broad, unanchored `.gitignore` rules** (plus one
  copy-time exclusion). Full inventory: `findings.md` §H.

| # | Artifact | Hidden by | State |
|---|---|---|---|
| 1 | `floyd-code/build/*.mjs` | root `build/` | **fixed** `9088cd4` |
| 2 | `apps/vis/web/src/components/logs/LogsTab.tsx` | root `logs/` | **fixed** `f8c4541` |
| 3 | `packages/agent-core-v2/src/human/credentials/*` | root `credentials` | **fixed** `f8c4541` |
| 4 | `migration-legacy/test/fixtures/**/.floyd/*` | root `.floyd/` | **fixed** `f8c4541` |
| 5 | `packages/klient/Dockerfile` | `floyd-code/.gitignore:26` bare `Dockerfile` | **open** |
| 6 | `apps/floyd-code/dist-web/**` (524 files) | copy exclusion + root `dist-web/` | **open** — blocks build gate |

Sweep proof (VERIFIED): `comm` of source-tree tracked files vs MEGAFLOYD tracked files under
`floyd-code/` = **525 paths = 524 × dist-web + 1 × klient/Dockerfile**. Nothing else is missing.

## Phases

### Track A — Reproducible tree (finish the P0 build proof)

| Phase | Task | Gate |
|---|---|---|
| A1 | Fix the ignore-rule defect class (instances 1–4) | `check-ignore` clean; files tracked | **done** `f8c4541` |
| A2 | Restore instance 5: force-add `packages/klient/Dockerfile` | file tracked; `git ls-files` shows it | open |
| A3 | Restore instance 6: copy `apps/floyd-code/dist-web/` from source, force-add | `check-web-assets.mjs` prints "Web assets OK" | open |
| A4 | Exhaustive "hidden artifact" sweep vs source tree | `comm` diff = ∅ beyond known exclusions | **done** (this plan) |
| A5 | Fresh-clone build proof: `pnpm install` → `pnpm -r run build` → `main.mjs --version` | exit 0 + version string | open |
| A6 | Record proof + update findings/progress/supercache; commit + push | gates table appended | open |

### Track B — The merge itself (P2–P5)

| Phase | Task | Gate |
|---|---|---|
| B1 (P2) | Context assembly in `agent-core-v2` — design + verified gap in `docs/B1-CONTEXT-ASSEMBLY.md`; gap is **git branch/status disclosure** (date is already handled by `dateChange` reminders) | typecheck + vitest; dumped prompt diffed against Go output |
| B2 (P3) | Port Go bash guards (no bare `<>`/`>|`/`&>|`, heredoc-in-substitution traps, read-before-write) to `externalHooks` rules | hook unit tests + denied-command demo |
| B3 (P4) | Identity + memory: `FLOYD.md` discovery + `.floyd/.supercache` read/injection + `FLOYD_*` markers | fresh-session smoke shows context loaded once, not per-turn |
| B4 (P5) | Package + ship: `floyd-one` launcher, own home dir, telemetry off | A/B session; no new egress beyond the inference host |

### Track C — Ecosystem surface (the "ultimate tool" reach)

Grounded in what already exists (from `floyd-code/AGENTS.md`): CLI/TUI, committed web UI
(`dist-web`), `vis` web GUI (sessions/replays), `floyd-inspect` (kap-server debug RPC),
vscode app, kap-server (REST+WS), remote-control tunnel, MCP gateway, slash commands.

| Phase | Task | Gate |
|---|---|---|
| C0 | **Discover** the actual plugin-dashboard / marketplace / MCP-gateway / slash-command code paths before designing (evidence, not assumption) | path inventory with `VERIFIED` tags |
| C1 | Wire the MCP gateway (MCP_SERVER_ROOM + secret-broker on inference) into the merged build | handshake succeeds; no secret in repo |
| C2 | Plugin dashboard + marketplace surface: catalog fetch + install path | install one plugin end-to-end in an isolated home |
| C3 | Slash commands + terminal + web GUI cohesion pass | one session exercised through all three surfaces |

Leads for C2 (UNVERIFIED, owner-supplied 2026-10-16): CDN "maybe" on Cloudflare; admin console
hosted by "indeeence". Do not treat as design inputs until confirmed by a run.

### Track D — Memory (answers the standing instruction)

| Phase | Task | Gate |
|---|---|---|
| D1 | Keep the tracked planning files + `.floyd/.supercache` checkpointed every session (already the practice) | each session ends with an updated checkpoint |
| D2 | Promote the "hold on to memory" wish into a product feature: `.supercache` auto-load + `FLOYD.md` (overlaps B3) | same gate as B3 |
| D3 | Decide where memory lives for the shipped tool (repo-local vs `~/.megafloyd`) | recorded in `docs/DECISIONS.md` |

### Track E — Company platform (IN build; owner directive 2026-10-16 — reverses D-1)

Service: `floyd-code/services/company-platform/` (Python stdlib console backend + member SPA).
Ship gate = **close D-1**: egress review, then add to the P5 build manifest.

| Phase | Task | Gate |
|---|---|---|
| E1 | First-run bootstrap so a clean checkout can start | clean-dir boot serves `/healthz` 200 **done** `491e1d7`; guarded `98db924` |
| E2 | De-hardcode internal endpoints + host-local paths (tool defaults, provider seed, install link, speech/ffmpeg/gateway/ComfyUI/llama-swap) | grep + gitleaks clean **done** `be0e4f0`, `86b7fb6` |
| E3 | Egress review: enumerate `gateway_bridge.mjs` / `model_access.py` relay / catalog fetch / feedback upload / media tools | **done** — enforced by `EgressReview` test `a3ae880` |
| E4 | Resume README gaps: portable-app command-window files; browser reply timing + reconnection checks | service-side done `cf976c6` (retry/backoff); command-window re-scoped to the app |
| E5 | Add service to the P5 build manifest; close D-1 | **done** — `ship-manifest.json` + verifier; D-1 = IN-shipping |

Tracks A/B/C/D below are unchanged and still pending except as noted (A2/A3/A5 landed in `62c0c0c`).

### Track F — README + GUI verticals (owner directive 2026-10-16; deliver at completion)

Spec: `docs/README-SPEC.md`; decision `docs/DECISIONS.md` D-7.

| Phase | Task | Gate |
|---|---|---|
| F1 | Confirm open points: which web GUI the verticals target; iPad standard size | recorded in the spec |
| F2 | Split that GUI into **three independent verticals** (Desktop autosize std↔widescreen; Tablet iPad; Mobile iPhone 16 Pro) — no shared-breakpoint interface | each vertical renders independently at its standard size |
| F3 | Capture real live screenshots: TUI + web GUI | images committed with alt text |
| F4 | Write `README.md` in the Floyd Labs brand voice | README present; spec gate met |

## Standing constraints

- Isolation: never point at `~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code`, `~/.floyd-10x`. Merged
  home dir = `~/.megafloyd`.
- Do not copy `credentials` from any install. Do not push secrets.
- `docs/prompt-pack/templates/` is the ONLY place templates are edited.
- Do not touch the source repos.

## Errors encountered (running log)

| Date | Error | Attempt | Resolution |
|---|---|---|---|
| 2026-10-16 | `Cannot find module .../floyd-code/build/raw-text-plugin.mjs` on clone | 1 | force-add helpers; root `build/` exception (`9088cd4`) |
| 2026-10-16 | `Could not resolve "../components/logs/LogsTab"` on clone | 1 | un-ignore four rule collisions (`f8c4541`) |
| 2026-10-16 | `check-web-assets.mjs`: missing committed `dist-web/` | — | open (A3) |
