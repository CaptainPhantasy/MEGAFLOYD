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

Portable pack (canonical) lives HERE, in this repo: docs/prompt-pack/
- templates/           = canonical copy of internal/agent/templates (edit here only)
- prompt.go.reference  = the Go assembly logic (spec, not to compile)
- MANIFEST.md          = interface + checksums + the real PromptDat struct
- hooks/               = reserved for ported hook definitions
Guard: `node scripts/verify-prompt-pack.mjs` (drift check) / `--sync` (push canonical -> Go tree).
The old staging copy at /Users/floyd/floyd-merge/prompt-pack/ is advisory and outside git.
The derived Go copy (floyd-go/internal/agent/templates/) is required by //go:embed and must
never be edited directly.

## Plug points on the Floyd Code side (from grep)

Paths verified 2026-10-16; all live under `floyd-code/packages/agent-core-v2/src/`.

| Task | File(s) to extend |
|---|---|
| Prompt/profile injection | `app/agentProfileCatalog/agentProfileCatalog.ts`, `app/agentProfileCatalog/profile-shared.ts` |
| System/context files (AGENTS.md today, add FLOYD.md + .supercache) | `workspace/workspaceAgentProfileLoader/internal/systemFile.ts`, `workspace/workspaceAgentProfileLoader/userAgentProfileLoaderService.ts` |
| Workspace instructions | `workspace/workspaceInstructions/workspaceInstructionsService.ts`, `agent/agentsMdReminder/agentsMdReminderService.ts` |
| Session bootstrap (git status/branch/commits, cwd, env markers) | `features/sessionInit/` |
| Tool discipline rules | `features/externalHooks/agent/agentExternalHooksService.ts`, `features/externalHooks/internal/types.ts` |
| Compaction/summary wording | `agent/fullCompaction/fullCompactionService.ts` (+ `compactionInstruction.ts`) |

## Phases

P0 inventory + build proof  [done]
- Floyd Code v2.0.2 runs; Go deployable tree builds (/tmp/floyd-deploy, 84 MB, exit 0); Go repo builds and tests green for internal/shell.

P1 portable prompt pack  [done: canonical pack + guard + manifest]
- templates are data; the canonical copy is `docs/prompt-pack/templates/`; the Go tree keeps a
  //go:embed-required derived copy. Drift guard: `scripts/verify-prompt-pack.mjs`.
- The real template interface is the `PromptDat` struct (`internal/agent/prompt/prompt.go:31`),
  NOT the variable names previously listed here (CWD/ENV/GIT_STATUS/... — none exist in the code).
  See `docs/prompt-pack/MANIFEST.md` §2 for the struct and §3 for per-template placeholders.
- gate: manifest + variable list reviewed; no code changes to Floyd Code yet.  [met]

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
