# Inside the source — what the intelligence is, and what was not connected

Date: 2026-10-22
Method: cloned `CaptainPhantasy/superfloyd-source` (snapshot of the working tree),
counted what is tracked, traced every prompt layer to its caller, then rebuilt.

## What the repository actually contains

| Area | Tracked files | Size | What it is |
|---|---:|---:|---|
| `floyd-go` | 1,489 | 13 MB | Go chassis: TUI, CLI, HTTP server, prompt engine, tools, skills |
| `floyd-code` | 4,531 | 143 MB | TypeScript shell: CLI, web server (`kap-server`), agent core (`agent-core-v2`), plugins |
| `desktop` | 29 | 360 KB | Electron wrapper, build/verify/smoke scripts |
| `docs` | 23 | 184 KB | Plans, decisions, prompt-pack, merge status |
| `scripts` | 7 | 48 KB | Build and packaging helpers |

The clone is byte-identical to the pushed tree (`03ff56a4…`), with one omission:
`.github/workflows/cloud-ci.yml`, because the push token has no `workflow` scope.

## The two prompt stacks, and the gap between them

**Go stack** — `floyd-go/internal/agent/`

- `prompt/prompt.go` (331 lines): assembles system prompt + git status + context files.
- `templates/` (14 files, ~910 lines): `protocol_kernel.md.tpl` (68), `floyd_protocol.md.tpl` (55),
  `superfloyd-coder.md.tpl` (48), `floyd-general.md.tpl` (70), `coder.md.tpl` (434),
  `initialize.md.tpl` (48), `task.md.tpl` (15), plus tool and agentic-fetch prompts.
- Callers: `ui/chat`, `app`, `backend`, `server`, `cmd` — the Go TUI and HTTP server only.
- `internal/megafloydweb` (26 files, the `--web` handoff) had **zero** references to
  `internal/agent` before this change.

**TypeScript stack** — `floyd-code/packages/agent-core-v2/`

- `app/agentProfileCatalog/system.md` (82 lines) — the only identity the web agent saw:
  "You are ${product_name}, an interactive general AI agent…".
- `systemPromptVars()` hardcoded `role_additional: ''` (`profile-shared.ts:147`), so the
  injection point existed and was never filled.
- `session/agentLifecycle/profile/profiles.ts` registers three profiles (`agent`, `coder`,
  `explore`) whose role text is 6 lines of handoff instructions, not an identity.
- 16 built-in skills, 11 core tools, 17 feature modules (cron, goal, plan, skill, swarm,
  todo, tower, usage…), and a hooks service wired to the tool executor.

**The gap:** `megafloydweb/command.go` spawns the Node server (`web --no-open --port --host`).
The Node server imports `@legacy-ai/agent-core-v2`, which renders `system.md`. Nothing ever
sends the Go layers across. Strings `protocol kernel` and `supercache` returned zero hits in
the TS tree.

## What the rebuild changed

1. `internal/agent/prompts.go`
   - Templates are no longer `go:embed`'ed as plain text. `scripts/obfuscate-prompts.mjs`
     emits `internal/agent/templates_blob.go` (gzip + base64); `mustDecodeTemplate` inflates
     them in memory at start-up and panics if a layer is missing.
   - `WebRolePrompt()` composes the three layers in session order: protocol kernel
     (with the output-style directive), FLOYD protocol, role template selected by binary
     name (`megafloyd` → SUPERFLOYD coder).
2. `internal/megafloydweb/env.go` + `home.go`
   - `BuildChildEnv` appends `FLOYD_ROLE_ADDITIONAL=gz1:<base64 gzip>`.
   - `decodeRolePrompt` exists so the wire format is testable.
3. `floyd-code/packages/agent-core-v2/src/app/agentProfileCatalog/profile-shared.ts`
   - `inheritedRoleAdditional()` decodes the payload once per process and is used when no
     explicit role is supplied; absent, malformed, or unknown-version values yield `''`.
4. Tests
   - Go: `TestBuildChildEnvCarriesRolePrompt` asserts tag, single entry, decoded content
     (`# Protocol Kernel`, `SUPERFLOYD`, >5 KB).
   - TS: `test/app/agentProfileCatalog/inheritedRoleAdditional.test.ts`, 4 cases — decode,
     reject bad input, render with inherited layers, explicit role wins.

## Verification actually run

| Check | Result |
|---|---|
| `go build ./internal/megafloydweb/` and `./...` | pass |
| `go test ./internal/agent/ ./internal/megafloydweb/` | pass |
| `strings bin/megafloyd \| grep "Protocol Kernel"` | 0 |
| `strings bin/megafloyd \| grep "SOTA Full-Stack Architect"` | 0 |
| `tsc -p packages/agent-core-v2` | 0 errors |
| `vitest run …/inheritedRoleAdditional.test.ts` | 4/4 pass |
| `node` decode of the Go payload | 9,498 bytes, contains kernel + SUPERFLOYD |
| `pnpm --filter @legacy-ai/floyd-code run build` | 17.82 MB `dist/main.mjs`, contains the env decode |
| `make build`, `make verify`, `make smoke` | 14/14, 3/3 pass |
| Engine `--web` with `MEGAFLOYD_WEB_SCRIPT` | server ready, HTTP 200 on the port |

`ps eww` no longer exposes environment for these processes on this macOS build, so the
parent→child environment was verified by unit test plus `handoff.go:221`
(`childEnv := BuildChildEnv(...)` → `SpawnOptions.Env`) rather than by process inspection.

## Still not connected

- **Go tools vs TS tools.** The Go agent has its own tool set (`internal/agent/tools`, MCP
  client, job runner); the web agent uses `agent-core-v2` tools. Only the prompt layer was
  bridged.
- **Go skills vs TS skills.** `internal/agent/ralph/commands` and the TS built-in catalog
  (16 entries) remain separate catalogs.
- **Hooks.** External hooks exist only in TS (`features/externalHooks`); no Go-side hook host.
- **Web UI source.** `apps/floyd-code/dist-web` is a compiled bundle; the UI source lives in
  a separate `code-app` repo that is not present anywhere on this machine.
- **CI workflow.** Not pushed (token scope), still only in the MEGAFLOYD origin repo.
- **Kimi lineage.** No `KimiSource`, no `moonshot` strings, zero history objects matching
  "kimi" — the parent source was never retained.
