# task_plan.md — MEGAFLOYD completion

## Goal

Bring the floyd-code ⇄ floyd-go merger at `/Users/floyd/MEGAFLOYD` to a **complete,
verified, committed P0/P1 state**, then hand P2–P5 back as a scoped roadmap:

- one prompt pack under version control (kill the 3-copy drift risk),
- `PLAN.md` plug-point table and P1 variable list corrected to the real code,
- P0 build proof reproducible on a fresh clone (exact toolchain preconditions),
- an explicit in/out decision for `services/company-platform/`,
- the source-repo secret gap recorded (not silently left).

Owner objective (2026-10-16, verbatim): **"complete MEGAFLOYD"**.

## Phase status

| Phase | Name | Status |
|-------|------|--------|
| 1 | Recon: read docs, run drift guard, verify paths/toolchain | complete |
| 2 | Prompt pack single-source-of-truth (F3) | complete |
| 3 | Correct `PLAN.md` plug-point table + P1 variable list (F2) | complete |
| 4 | README reproduce block: exact node/pnpm/go + GOTOOLCHAIN (F5) | complete |
| 5 | `company-platform` in/out decision (F6) | complete — `docs/DECISIONS.md` D-1 = OUT |
| 6 | Source-repo secret gap recorded (F1 caveat) | complete — `docs/DECISIONS.md` D-2 |
| 7 | Run gates: guard + `go build/test` + floyd-code build | complete — all green |
| 8 | Commit verified state; write progress | complete |

## Gate results (2026-10-16)

| Gate | Command | Result |
|---|---|---|
| Prompt-pack drift | `node scripts/verify-prompt-pack.mjs` | **PASS** |
| Floyd Code runs | `node floyd-code/apps/floyd-code/dist/main.mjs --version` | **2.0.2** |
| Go builds | `go build ./...` | **exit 0** |
| Go shell tests | `go test ./internal/shell/...` | **ok 4.890s** |
| Go vet (agent) | `go vet ./internal/agent/...` | **clean** |

**P2–P5 (the merge itself: context assembly, hook port, identity/memory, package+ship)
are OUT of this plan's executing scope** — they are engineering, not closure. Listed in
"Remaining roadmap" below; not to be silently claimed done.

## Standing constraints

- `README.md` isolation rules hold: never point at `~/.floyd7`, `~/.floyd-ff`,
  `~/.floyd-code`, `~/.floyd-10x`. Merged home dir = `~/.megafloyd`.
- Do not copy `credentials` from any install. Do not push secrets.
- `docs/prompt-pack/templates/` is the ONLY place templates are edited.
- Do not touch the source repos (`floyd-v6/src`, `floyd-v6/KimiSource/kimi-code`).

## Remaining roadmap (P2–P5, unscoped here)

- P2 context assembly service in `agent-core-v2` (write against the real `PromptDat` struct).
- P3 tool-discipline hooks ported to `externalHooks`.
- P4 identity + memory: `FLOYD.md` + `.floyd/.supercache` injection.
- P5 package + ship: `floyd-one` launcher, `~/.megafloyd`, telemetry off.

## Errors encountered

| Error | Attempt | Resolution |
|-------|---------|------------|
| — | — | — |
