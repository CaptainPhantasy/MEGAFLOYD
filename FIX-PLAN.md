# FIX-PLAN — remediation of the 2026-10-19 audit

Status: agent process document (not law). Maps every audit item to a lane.
One worktree per lane. Lanes with shared files are sequenced, never parallel.

## Wave 1 (parallel — disjoint file ownership)
- A1 fix-skills: audit 1, 8 — skills load at runtime from the data dir; embed
  + install at startup (like ralph); alias resolution from skills.json.
  Owns: internal/commands installer + new files; app.go call site.
- A2 fix-cli: audit 2, 9, 17(partial) — register all 11 cmd subcommands;
  megafloyd naming (usage/help/version); identity in root/main.
  Owns: main.go, internal/cmd/.
- A3 fix-tools: audit 3 — register selfheal, progress, vision tools;
  verify allowlist + count. Owns: coordinator.go, tools/, config allowlist.
- A4 fix-style: audit 13 — output-simplification engine, harness-level,
  always on. Owns: templates/protocol_kernel + new internal/megafloyd/style.

## Wave 2 (sequential after wave 1 merges)
- B1 fix-slash: audit 17 — 43 core slash triggers wired to real actions.

- B2 fix-jobs: audit 18, 19 — job-record system: contract, prove, recover,
  drift, handoff, replay, why; MCP/plugin prompt XML population.
- B3 fix-auth: audit 11 — wire megafloydauth: login flow, token store.
- B4 fix-remote: audit 12, 15, 27 — web handoff (existing bundle, node server,
  tailscale-aware bind/hosts/banner), resume --web. (P8 mechanics implemented
  as engineering for the approved cloud/mobile requirements.)
- B5 fix-admin: audit 14 — admin client: users, invites, grants (read-only
  discovery first; no invented endpoints).
- B6 fix-cloud: audit 10, 24 — devcontainer version stamp; execute VM
  installer in a container; linux smoke. Push to GitHub only with owner OK.
- B7 fix-memory: audit 16 — app-side supercache checkpoint writer.
  Owns: app.go (after A1 merge).

## Wave 3
- Integration: full gates, rebuild local binary, smoke, docs/SSOT status,
  self-review with evidence per item.
