# SSOT — MEGAFLOYD

Single source of truth for the megafloyd project. Owner-ratified 2026-10-18.
This file supersedes all prior planning documents in this repo
(`task_plan.md`, `PLAN.md`, `HANDOFF.md`, `progress.md`, `findings.md` are
historical, non-binding, and kept only as reference).

## 1. Product definition (owner's words, binding)

- One new entity, launched as **`megafloyd`**. Its own command, its own home
  dir (`~/.megafloyd`), its own package. Replaces nothing — `floyd-v6`,
  `superfloyd`, `ff`, and every existing command stay untouched.
- The most modern chassis carrying **all features from every verified
  instance**, with the **superfloyd v5.3 intelligence layer** as its mind.
- Installs on owner's machines AND in a cloud VM / GitHub Codespace.
  Operable from a phone on cell signal alone, working on GitHub repos, with
  **zero dependence on home machines, home network, or home power** (rural
  outages are the driving constraint).
- No upstream connection: this is a severed Crush fork. Never fetch, pull, or
  push any charmbracelet remote. Never redistribute (FSL-1.1-MIT, internal
  Legacy AI fleet only).

## 2. Canonical sources

| Role | Source | Status |
|---|---|---|
| Chassis (host) | `floyd-go/` in this repo (Crush v0.92.0 rebrand, go 1.26) | builds clean, VERIFIED |
| Intelligence layer donor | `/Volumes/Storage/floyd` (v5.3 superfloyd, branch v6-canonical) | read-only |
| Remote | `github.com/CaptainPhantasy/MEGAFLOYD` (private) | only remote, ever |
| Rejected donors | STABLE MCP copy, floyd-main, floyd-next, v5 backups, .codex-tmp, Floyd_Code-3.7, GO2TUI copy | superseded |

## 3. Expected feature list (owner-ratified)

### A. Chassis features (already in floyd-go v0.92 — keep, do not break)
1. Bubble Tea v2 TUI with command palette (ctrl+p), themes, fuzzy filter
2. Multi-provider LLMs: Anthropic, OpenAI, Gemini, Bedrock, Copilot, Hyper,
   MiniMax, Vercel, OpenAI/Anthropic-compatible endpoints
3. Client/server split: `serve` mode (Unix socket/TCP), multi-client
   workspaces sharing sessions/permissions/LSP/MCP, SSE events
4. Claude Code-compatible hooks engine (PreToolUse), hook-gated tools
5. MCP client integration; LSP client manager with diagnostics/references
6. Agent Skills open standard (SKILL.md discovery)
7. floydrc bash config + floyd.json; config-as-service
8. SQLite session persistence; permission allow/deny lists; yolo mode
9. Vision: clipboard image paste, file picker, drag-drop, LM Studio vision
   discovery, model-gated image delivery, Kitty/block terminal rendering
10. Background shell jobs (job_output/job_kill)
11. Telemetry OFF by default (opt-in via MEGAFLOYD_POSTHOG_KEY; upstream
    PostHog key already stripped)

### B. Intelligence layer (ported from superfloyd v5.3 — waves 1-3)
12. Ralph self-correction loop (+ ralph install/status/cancel commands)
13. Swarm coordinator (multi-agent dispatch)
14. Ghost-text user-voice suggestions while typing
15. Prompt stash
16. Token budget guardrails + token telemetry
17. Agent prompt cache; context assembly; model selector; summarizer
18. Workflow engine (definitions, steps, checkpoints)
19. Plugins system + extensibility libraries (agents/skills/plugins dialogs)
20. Agent library loader; skills runtime
21. Session fork; permission profiles; file write tracker
22. Paranoia (prompt-injection defenses); validation; error formatter
23. 21 tools: apply_unified_diff, archive_query, complexity_estimator,
    context_status, failure_mode_enumerator, get_active_diff, lab,
    list_symbols, manage_scratchpad, mcp_task_status, parallel_bash,
    progress, project_map, refactor_blueprint, registry, selfheal,
    smart_replace, symbol_index, transcript_export, vision, workflow
24. Superfloyd resilience + scoreboard commands; doctor; ai/prompt commands;
    codebase analysis CLI
25. UI extras: config audit dialog, MCP dialog, rename session, diffview
    chroma syntax, terminal component/session, prompt usage display
26. Node MCP bridges wired as config: room, steer, supercache, terminal

### C. Packaging & delivery (wave 4)
27. `megafloyd` launcher binary + own home dir; MEGAFLOYD_* env namespace;
    zero collision with existing floyd commands
28. Local builds: macOS arm64 (+ amd64), linux amd64/arm64
29. Cloud install: one-command script + devcontainer for Codespace/VM
30. Phone-reachable surface served from the cloud instance (web front over
    serve mode; exact surface chosen by code evidence in wave 4)
31. MCP room endpoint in config: public URL when cloud, tailnet URL at home
    (public endpoint published by owner under floydslabs.com)
32. FLOYD.md identity + workethic loaded at startup; memory checkpoint files

## 4. Build waves (each: own worktree, build+test+lint gates, semantic commits)

- Wave 1 — leaf/utility packages (19 files): RUNNING
- Wave 2 — agent layer: ralph, 21 tools, workflow, plugins, extensibility
- Wave 3 — UI dialogs + cmd commands
- Wave 4 — integration wiring + megafloyd packaging + cloud path
- Gate between waves: `go build ./...`, `go test -race ./internal/...`,
  `gofumpt`, commit per package cluster. No wave starts before the prior
  wave merges to main.

## 5. Operating rules (binding on every agent on this project)

1. Evidence tags: VERIFIED (ran it) / HYPOTHESIS / LEAD. Never assert state
   without the command and its output.
2. Donor repos are read-only. All work lands in this repo.
3. Never weaken permission checks. Never commit secrets or credentials.
4. Three-strike rule: three failed attempts on one defect = stop and report.
5. Memory: update this SSOT + progress notes at every wave boundary.
6. Owner gate: waves 2-4 do not start until the owner ratifies §3.

## 6. Status log

- 2026-10-18: SSOT created. Chassis build verified. Feature inventory complete
  (84 Go files + 10 test files to port). MCP room ownership confirmed on
  inference box (`/srv/organized/services/MCP_SERVER_ROOM`, LAN git remote,
  owner handling the public publish).
- 2026-10-18: WAVE 1 COMPLETE on branch `port/wave1-utilities` — 13 new
  packages + 5 additive files + 1 test, 14 commits; `db/embed.go` skipped
  (host already has identical embed). Gates re-verified independently:
  build exit 0, tests ok, tree clean. Awaiting owner ratification of §3
  before wave 2.
