# SSOT — MEGAFLOYD

Single source of truth for the megafloyd project. Owner-ratified 2026-10-18.
This file supersedes all prior planning documents in this repo
(`task_plan.md`, `PLAN.md`, `HANDOFF.md`, `progress.md`, `findings.md` are
historical, non-binding, and kept only as reference).

## 0. Provenance and law-making (OWNER-DIRECTIVE 2026-10-19)

Every rule in this file carries a tag:
- **[OWNER-DIRECTIVE]** — the owner said it, in conversation, and it was
  written down as law. Nothing becomes this tag silently; if there is any
  chance of drift, the agent ASKS before writing it, regardless of any
  "don't ask questions" instruction. This overrides all other rules.
- **[AGENT-PROPOSAL]** — the agent's engineering or process choice. Never
  constrains the product. The owner may strike or change any of these at
  any time without notice; the agent adapts.
Passing conversation does NOT become law. If the agent is unsure whether
something the owner said is a directive or a remark, it asks. Clear
separation between owner intent and agent implementation is mandatory.

## 1. Product definition ([OWNER-DIRECTIVE] — owner's words, binding)

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
27. [OWNER-DIRECTIVE 2026-10-19] Output-simplification engine: the agent is
    forced to simplify every output — short lines, plain words, decision
    first, at-a-glance readable on a phone. Not because the owner is
    non-technical; because he runs many sessions and his cognitive
    bandwidth must not be spent decoding prose or buzzwords. Never make him
    ask for a repeat.

### C. Packaging & delivery (wave 4)
28. `megafloyd` launcher binary + own home dir; MEGAFLOYD_* env namespace;
    zero collision with existing floyd commands
29. Local builds: macOS arm64 (+ amd64), linux amd64/arm64
30. Cloud install: one-command script + devcontainer for Codespace/VM
31. Phone-reachable surface served from the cloud instance (web front over
    serve mode; exact surface chosen by code evidence in wave 4)
32. MCP room endpoint in config: public URL when cloud, tailnet URL at home
    (public endpoint published by owner under floydslabs.com)
33. FLOYD.md identity + workethic loaded at startup; memory checkpoint files
34. Auth-server identity: megafloyd authenticates against the owner's auth
    server (the f7 model, inference-hosted OAuth) — not raw local API keys
35. Admin plane: invite + accept new users, grant/revoke access to company
    tools and models (owner-attested; verify console in wave 4)
36. Company tools + models access control inherited by cloud and phone
    sessions — sign in from a Codespace exactly like at home

### D. Command surface [OWNER-DIRECTIVE 2026-10-19]

Owner directive: slash-command system modeled on the floyd7 harness method;
superfloyd tools are FIRST autonomous registry tools, slash aliases optional;
extensible via /skill:<name> and /plugin:<command>. The full extracted
command name lists (43 floyd7 commands, superfloyd aliases, CLI subcommands)
are staged in PROPOSALS.md as P6 awaiting line-item confirmation.

1. Modern core (modeled on the floyd7 harness, TS v2.0.2 — the ideal):
   /yolo /auto /permission /settings /plan /model /secondary-model /effort
   /provider /help /new /sessions /tasks /mcp /plugins /add-dir /experiments
   /reload /reload-tui /compact /init /fork /title /usage /status /undo
   /editor /theme /login /logout /export-md /export-debug-zip /copy /exit
   /version /swarm /goal /btw /web /remote-control /desktop /feedback
2. Superfloyd intelligence shortcuts — NOTE: every item below is FIRST a
   built-in agent tool registered in the tool registry with its description
   file (model-invoked, autonomous — SSOT rule 0). The slash is only an
   optional manual alias for the owner: /ralph /ralph-status /ralph-cancel
   /selfheal /parallel-bash /workflow /project-map /symbol-index
   /transcript-export /archive-query /scratchpad /context-status /budget
   /scoreboard /doctor /prompt /ai /stash /ghost /vision /lab /resilience
3. Extensible: /skill:<name>, /<plugin>:<command>, user markdown commands,
   MCP prompts.

CLI subcommands: run (default TUI), serve, login, logout, models, projects,
schema, stats, logs, session, update-providers, ai, codebase, doctor, exec,
fileops, prompt, scoreboard, resilience.

Implementation notes: group 1 names register as true slash commands in the Go
chassis (many already exist as palette actions; /goal /tower-class /btw /web
/remote-control need new server-side work in wave 4). Group 2 arrives with
waves 2–3 and gets registered in the command registry. Group 3 uses the
chassis markdown-command + skills + MCP-prompt loaders.


## 4. Build waves (agent process — changeable, not law)

- Wave 1 — leaf/utility packages (19 files): DONE, merged to main
- Wave 2 — agent layer: ralph, 21 tools, workflow, plugins, extensibility:
  RUNNING (worktree port/wave2-agent)
- Wave 3 — UI dialogs + cmd commands
- Wave 4 — integration wiring + megafloyd packaging + cloud path
- Gate between waves: `go build ./...`, `go test -race ./internal/...`,
  `gofumpt`, commit per package cluster. No wave starts before the prior
  wave merges to main.

## 5. Owner directives

0. [OWNER-DIRECTIVE 2026-10-19] Tools are agent-autonomous by default: every
   ported tool registers in the agent tool registry with its description
   file, so the model decides when to use it. Slash commands are manual
   overrides for the owner — never the primary interface.
1. [OWNER-DIRECTIVE — safety] Donor repos are read-only. Never weaken
   permission checks. Never commit secrets or credentials.
2. [OWNER-DIRECTIVE] No upstream connection: never fetch, pull, or push any
   charmbracelet remote; never redistribute.
3. [OWNER-DIRECTIVE 2026-10-19] The SSOT contains only what the owner has
   been absolutely clear about. Agent proposals are presented in
   PROPOSALS.md and championed for entry — never inserted by default.
4. [OWNER-APPROVED 2026-10-19, formerly PROPOSALS P1-P6] Process rules now
   binding: P1 evidence tags (VERIFIED/HYPOTHESIS/LEAD) on state claims;
   P2 three-strike stop-and-report on repeated failure; P3 SSOT + progress
   updated at every wave boundary; P4 build+test+lint gate green before the
   next wave; P5 per-feature worktrees for parallel agents; P6 the approved
   command-name lists (43 floyd7 slash commands, superfloyd manual aliases,
   CLI subcommands) as the shipped command surface.

## 6. Status log

- 2026-10-18: SSOT created. Chassis build verified. Feature inventory complete
  (84 Go files + 10 test files to port). MCP room ownership confirmed on
  inference box (`/srv/organized/services/MCP_SERVER_ROOM`, LAN git remote,
  owner handling the public publish).
- 2026-10-18: WAVE 1 COMPLETE on branch `port/wave1-utilities` — 13 new
  packages + 5 additive files + 1 test, 14 commits; `db/embed.go` skipped
  (host already has identical embed). Gates re-verified independently:
  build exit 0, tests ok, tree clean.
- 2026-10-19: WAVE 2 COMPLETE and merged — ralph (loop + install + 3
  commands), 21 tools registered via coordinator.buildTools + allowed in
  default coder agent + 27 description .md files, workflow engine, plugins,
  extensibility, skills dedup, 3 test files. 12 commits, +7,520/−182.
  Gates green (build 0, race-tests 0 FAIL, gofumpt clean); independently
  re-verified (build, registration greps, ralph wiring) before merge.
  Known cosmetic churn: gofumpt reformatted 137 pre-existing chassis files.
  WAVE 3 dispatched (UI dialogs, slash layer, ghost-text, CLI commands);
  pre-merge check against approved P6 list required.
- 2026-10-19: Owner approved P1-P6; process rules promoted into §5.4.

### E. Full skill mandate [OWNER-DIRECTIVE 2026-10-19]

Every named command from the owner's 2026-10-19 harness-research document
becomes a real skill in megafloyd, triggered by its name:

/btw /compact /context /rewind /branch /fork /plan /loop /remote-control
/effort /fast /mcp /doctor /bisect /handoff /arena /race /invariants
/redteam /shadow /blast-radius /freeze /thaw /checkpoint /restore
/spec-sync /route /retro /contract /prove /recover /budget /drift
/reproduce /replay /why /compare /promote /learn /test /lint
/security-review /debug

Combining rule: when two skills' work overlaps 90%+, build ONE skill that
answers to both names (e.g. /branch + /fork). No skill may be replaced by
"something roughly equivalent" — the named trigger must do the named job.

### F. Web/TUI handoff fixes [OWNER-DIRECTIVE 2026-10-19]

Three verified bugs in the existing f7 web/TUI handoff MUST be fixed in
megafloyd so they never recur:

1. Tailscale-ready web UI. At web-server start, auto-detect the Tailscale
   IPv4 (`tailscale ip -4`) and MagicDNS hostname (`tailscale status --json`
   .Self.DNSName). Add both (`*.ts.net` and `100.x.x.x`) to the server's
   allowed-hosts allowlist by default (no more 40301 Invalid Host header).
   Bind loopback + Tailscale interface (or 0.0.0.0 in --remote/--tailscale
   mode). Print the clickable https://<magicdns>/sessions/...#token= URL in
   the banner instead of 127.0.0.1.
2. Terminal teardown. On every TUI exit/panic/handoff path, write
   "\x1b[<u\x1b[>4;0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?25h"
   to stdout before exit (disables Kitty CSI-u + mouse modes, shows cursor).
   Spawned web server must have stdin redirected to /dev/null so the pane
   returns to a usable prompt.
3. Resume into web mode. `<cli> -r <session_id> --web` starts/reuses the
   background web server on a deterministic port, registers the Tailscale
   route, and prints the ready-to-click Tailscale URL.
