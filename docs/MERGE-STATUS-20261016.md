# MEGAFLOYD — Merge Status Report

**Date:** 2026-10-16
**Repo:** `/Users/floyd/MEGAFLOYD` @ `e33af3f` (main, `origin=github.com/CaptainPhantasy/MEGAFLOYD`)
**Scope:** state of the floyd-code ⇄ floyd-go merger against the claims in `README.md` and `PLAN.md`.
**Method:** read-only inspection + source-of-truth diffing. No builds were executed (see F5).

---

## 1. Verified state

| Claim (README/PLAN) | Verified against | Result |
|---|---|---|
| `floyd-code/` = copy of `floyd-v6/KimiSource/kimi-code`, `.git`/`node_modules`/native+web dists excluded | `diff -rq` (excl. `.git`, `node_modules`, `.turbo`) | **TRUE** — only diff is the excluded set (`.tmp/live-*.log`, `apps/floyd-code/dist-native`, `apps/floyd-code/dist-web`) |
| Floyd Code version 2.0.2 | `floyd-code/apps/floyd-code/package.json` | **TRUE** (`"version": "2.0.2"`; root pkg is `@legacy-ai/monorepo` 0.1.1, private) |
| `floyd-go/` = copy of `floyd-v6/src`, `.git` excluded | `diff -rq` | **TRUE except one file** — only `internal/event/event.go` differs (see F1) |
| Go source HEAD `57ba00a8` | `git -C floyd-v6/src log --oneline -1` | **TRUE** (`57ba00a8 fix: prompt override test …`) |
| Go prompt assembly = `prompt.go` (301 lines) | `wc -l` | **TRUE** (301) |
| `docs/prompt-pack/` = templates extracted as plain data | `diff -rq` vs `floyd-go/internal/agent/templates` and vs source | **TRUE, byte-identical** — see F3 |
| Plug-point paths in `PLAN.md` | path existence checks | **PARTLY FALSE** — 5 of 9 paths wrong (see F2) |
| No credentials tracked; secret reviewed | `git ls-files` + `.gitleaks.toml` | **TRUE for the reviewed set** — see F1 |
| Repo size / tree | `du`, `git ls-files` | 139 MB total; 113 MB `floyd-code`, 11 MB `floyd-go`, 60 KB `docs`; 5 057 tracked files (3 421 TS/TSX, 608 Go, 224 Go tests) |

Phase gates declared in `PLAN.md`: P0 **done**, P1 **staged**, P2–P5 **not started** (nothing in the tree implements them; see F4).

---

## 2. Findings

### F1 — Telemetry key remediation is real and correctly scoped
`floyd-go/internal/event/event.go` no longer hardcodes the PostHog project key present in the source:
- source `floyd-v6/src/internal/event/event.go:18` → `key = "phc_4zt4VgDWLqbYnJYEwLRxFoaTL2noNrQij0C6E8k3I0V"`
- merged `floyd-go/internal/event/event.go:19-23` → `telemetryKeyEnv = "MEGAFLOYD_POSTHOG_KEY"`, client init reads the env var and logs `"PostHog telemetry disabled"` when unset.

`.gitleaks.toml` allowlists exactly four reviewed paths (doc examples + one test fixture) and is not broad. **Impact:** positive — this is the one intentional source divergence and it is documented. Remaining caveat: the key still lives in the *source* repo `floyd-v6/src` (outside this project).

### F2 — `PLAN.md` plug-point table is stale (5/9 paths wrong)
The "Plug points on the Floyd Code side" table points at directories that do not exist. Actual locations:

| PLAN says | Actual path |
|---|---|
| `features/workspaceInstructions/` | `packages/agent-core-v2/src/workspace/workspaceInstructions/workspaceInstructionsService.ts` |
| `features/agentsMdReminder/` | `packages/agent-core-v2/src/agent/agentsMdReminder/agentsMdReminderService.ts` |
| `features/fullCompaction/` | `packages/agent-core-v2/src/agent/fullCompaction/fullCompactionService.ts` (+ `compactionInstruction.ts`) |
| `features/externalHooks/agentExternalHooksService.ts` | `packages/agent-core-v2/src/features/externalHooks/agent/agentExternalHooksService.ts` |
| `workspace/userAgentProfileLoaderService.ts` | `packages/agent-core-v2/src/workspace/workspaceAgentProfileLoader/userAgentProfileLoaderService.ts` |

Confirmed OK: `app/agentProfileCatalog/{agentProfileCatalog.ts,profile-shared.ts}`, `workspaceAgentProfileLoader/internal/systemFile.ts`, `features/sessionInit/`, `features/externalHooks/internal/types.ts`.
**Impact:** P2/P3/P4 todos would start from wrong line-1 context. Fix the table before any wiring.

### F3 — the "prompt pack" is a byte-identical duplicate, in two places
`docs/prompt-pack/templates/*` is identical to `floyd-go/internal/agent/templates/*` (8 files, 615 lines; `coder.md.tpl` is 434) **and** to the source templates. `docs/prompt-pack/prompt.go.reference` is byte-identical to `floyd-go/internal/agent/prompt/prompt.go` (301 lines, `diff` empty). A second copy also exists outside the repo at `/Users/floyd/floyd-merge/prompt-pack/` (identical; `hooks/` is empty).
**Impact:** PLAN P1 says "treat [coder.md.tpl] as the source of truth, version it in the pack" — with three unreconciled copies there is no single source of truth, and any edit silently forks behaviour. This is the top prompt-drift risk.

### F4 — no ported behaviour exists in the Floyd Code side yet
`grep -rn "supercache|FLOYD.md"` over `packages/ apps/` returns **nothing**. `systemFile.ts` is still AGENTS.md-only (`userAgentProfileLoaderService.ts:17` imports `loadSystemMdProfile`; `workspaceInstructionsService.ts:133` only knows AGENTS.md). **Impact:** P2/P3/P4 are genuinely unstarted; the tree confirms it, so "copied, not merged" in README is accurate.

### F5 — build proof is not reproducible in the current tree
`floyd-code/node_modules` is absent (expected — gitignored) so P0's "Floyd Code v2.0.2 runs" cannot be re-verified without `pnpm install` (needs Node ≥ 24.15.0 / pnpm 10.33.0). Local `go version` is **go1.25.7**, but `floyd-go/go.mod` declares `go 1.26.6`, so any `go build ./...` silently downloads a toolchain (GOTOOLCHAIN).
**Impact:** P0's gate is an assertion in `PLAN.md`, not something a fresh clone can reproduce. Add the exact verification commands to the README (they are present, but the version/toolchain precondition is not).

### F6 — minor housekeeping
- `floyd-go/.envrc` (`use flake`) is tracked. Harmless (flake devshell, no secrets) but it is the only `.env*`-ish file in the index.
- `floyd-go/.github/` and `.gitignore`/`.gitattributes` were copied in with the source tree; they are now dead config inside a monorepo that already has its own `floyd-code/.github/`.
- `floyd-code/services/company-platform/` (Python + JS "company platform") was copied in wholesale; PLAN's P5 promises "no new network egress beyond the existing inference host" — this service needs an explicit in/out decision before P5.

---

## 3. Gate readiness

| Phase | Declared | Reality on disk |
|---|---|---|
| P0 inventory + build proof | done | Inventory verifiable; build proof not reproducible (F5) |
| P1 portable prompt pack | staged | Staged **three times** (F3); no manifest, no variable list committed |
| P2 context assembly | not started | Confirmed absent (F4) |
| P3 tool discipline hooks | not started | Source guards exist in `floyd-go/internal/{hooks,shell/redirect.go}`; no port to `externalHooks` |
| P4 identity + memory | not started | No `.supercache` / `FLOYD.md` support anywhere |
| P5 package + ship | not started | `floyd-one` launcher absent; `~/.megafloyd` not created |

---

## 4. Recommended next actions (in order)

1. Rewrite the `PLAN.md` plug-point table from F2; delete the copy that `docs/prompt-pack` or `floyd-merge/prompt-pack` duplicates, keeping exactly one under version control (F3).
2. Write the P1 deliverable that is actually missing: `docs/prompt-pack/MANIFEST.md` with the variable list (CWD, ENV, GIT_STATUS, GIT_BRANCH, RECENT_COMMITS, FLOYD_MD, SUPERCACHE, SKILLS, MCP_PROMPTS, IDENTITY) and a checksum of each template, so drift is detectable.
3. Add a reproduce block to `README.md`: exact `node -v`/`pnpm -v`/`go version`, plus `GOTOOLCHAIN` behaviour, so P0's proof is re-runnable on a fresh clone.
4. Decide in/out for `services/company-platform/` before P5 egress review (F6).
5. Re-run `.gitleaks` against the source repo `floyd-v6/src` — the PostHog key was fixed here but not there (F1).
