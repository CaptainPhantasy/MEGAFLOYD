# Floyd merge — porting the Go behaviour layer into Floyd Code (floyd7 / f7)

Goal: one build = Floyd Code's shell (auth, servers, web GUI, editors) + the Go floyd's
agent behaviour (prompt/context assembly, identity, tool discipline, hooks, guards).

## Verified targets (2026-10-16)

Floyd Code (base):
- source: /Users/floyd/floyd-v6/KimiSource/kimi-code   (TS/pnpm monorepo, runs `node apps/floyd-code/dist/main.mjs`, version 2.0.2)
- launcher: /Users/floyd/.local/bin/floyd7 -> ${FLOYD_COMPANY_INSTALL_ROOT:-$HOME/.local/share/floyd7-company}/bin/f7 ; home dir ~/.floyd7
- already has: externalHooks runner, agent profiles, AGENTS.md/systemFile loader, session init, compaction, oauth, kap-server/acp-server, vis web GUI, vscode app
- missing: .supercache, FLOYD.md, the Go protocol/system prompt, tool-discipline rules

Go floyd (behaviour source):
- repo: /Users/floyd/floyd-v6/src  (HEAD 57ba00a8)
- prompt assembly: internal/agent/prompt/prompt.go (301 lines)
- prompt assets: internal/agent/templates/ (coder.md.tpl 21.5 KB, initialize.md.tpl, summary.md, task.md.tpl, title.md, agent_tool.md, agentic_fetch*.md)
- skills: internal/skills (builtin + catalog)
- hooks: internal/hooks/{hooks.go,input.go,runner.go} — events PreToolUse, PostToolUse, SessionStart, Stop; decisions allow/deny/halt
- guards added this session: internal/shell/redirect.go (validateRedirs), recover in background goroutine

Portable pack staged here: /Users/floyd/floyd-merge/prompt-pack/
- templates/        = copy of internal/agent/templates
- prompt.go.reference = copy of the Go assembly logic (spec, not to compile)
- hooks/            = reserved for ported hook definitions

## Plug points on the Floyd Code side (from grep)

| Task | File(s) to extend |
|---|---|
| Prompt/profile injection | packages/agent-core-v2/src/app/agentProfileCatalog/{agentProfileCatalog.ts,profile-shared.ts} |
| System/context files (AGENTS.md today, add FLOYD.md + .supercache) | packages/agent-core-v2/src/workspace/workspaceAgentProfileLoader/internal/systemFile.ts , userAgentProfileLoaderService.ts |
| Workspace instructions | packages/agent-core-v2/src/features/workspaceInstructions/ , agentsMdReminder/ |
| Session bootstrap (git status/branch/commits, cwd, env markers) | packages/agent-core-v2/src/features/sessionInit/ |
| Tool discipline rules | packages/agent-core-v2/src/features/externalHooks/{agentExternalHooksService.ts,internal/types.ts} |
| Compaction/summary wording | packages/agent-core-v2/src/features/fullCompaction/ (+ templates summary.md) |

## Phases

P0 inventory + build proof  [done]
- Floyd Code v2.0.2 runs; Go deployable tree builds (/tmp/floyd-deploy, 84 MB, exit 0); Go repo builds and tests green for internal/shell.

P1 portable prompt pack  [staged, needs wiring]
- keep templates as data; define variables: CWD, ENV, GIT_STATUS, GIT_BRANCH, RECENT_COMMITS, FLOYD_MD, SUPERCACHE, SKILLS, MCP_PROMPTS, IDENTITY.
- gate: manifest + variable list reviewed; no code changes to Floyd Code yet.

P2 context assembly in Floyd Code
- new service in agent-core-v2 that fills the pack at session start, reusing sessionInit for git data.
- gate: pnpm typecheck + vitest pass; assembled prompt dumped and diffed against the Go output.

P3 tool discipline as hooks
- port bash guards (no bare `<>`/`>|`/`&>|`, no heredoc-in-command-substitution traps, read-before-write) to externalHooks rules.
- gate: hook unit tests + a denied-command demo.

P4 identity + memory
- FLOYD.md discovery (project + global), .floyd/.supercache read/injection, env markers (FLOYD_*).
- gate: fresh-session smoke test shows context loaded once, not duplicated per turn.

P5 package + ship
- build: pnpm -r build ; launcher `floyd-one` (own home dir, e.g. ~/.floyd-one), telemetry disabled by default.
- gate: A/B session on the same task; no new network egress beyond the existing inference host.

## Risks / non-negotiables
- Two languages and two session stores: never point the merged build at ~/.floyd7 or ~/.floyd-ff at the same time as another build.
- Prompt drift: the Go coder.md.tpl is the behaviour I run on; treat it as the source of truth, version it in the pack.
- Egress: keep FLOYD_DISABLE_TELEMETRY=1; the f7 launcher already sets it; do not add endpoints.
- Auth: ~/.floyd7/credentials must not be copied into the merged home dir.
