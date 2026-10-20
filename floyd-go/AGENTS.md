# Floyd Development Guide

## Project Overview

Floyd is a terminal-based AI coding assistant written in Go. It connects to
LLMs and gives them tools to read, write, and execute code. It supports many
providers (Anthropic, OpenAI, Gemini, Bedrock, Copilot, Hyper, MiniMax,
Vercel, and more via OpenAI-/Anthropic-compatible APIs), integrates with LSPs
for code intelligence, and is extensible through MCP servers, hooks, and
Agent Skills.

**This repository is a private Legacy AI rebrand of
[Charm Crush](https://github.com/charmbracelet/crush) v0.92.0
(FSL-1.1-MIT).** It is for internal use only and must never be redistributed
or published. Upstream engine behavior is unchanged unless documented in this
repo. The fork has no upstream link: `origin` is the private
`github.com/CaptainPhantasy/MEGAFLOYD` repository, and there is no remote
pointing at `charmbracelet/crush`.

Key facts:

- Module path: `github.com/legacy-ai/floyd` (renamed from
  `github.com/charmbracelet/crush`).
- Go 1.26; builds with `CGO_ENABLED=0` and `GOEXPERIMENT=greenteagc`.
- Active branch: `floyd-v6`.
- SQLite persistence via `ncruces/go-sqlite3` / `sqlc`; no CGO.

### Rebrand conventions

A repo-wide sweep renamed user-facing identifiers from Crush to Floyd. When
touching user-facing strings, env vars, file names, or docs, keep the Floyd
naming and respect the established aliases:

- Env vars: `FLOYD_*` (e.g. `FLOYD_GLOBAL_CONFIG`, `FLOYD_GLOBAL_DATA`,
  `FLOYD_PROFILE`, `FLOYD_IDENTITY`). `CRUSH_*` names are kept as
  compatibility mirrors; check the allow-list in the config package before
  adding or removing mirrors.
- Files and paths: `floydrc`, `.floydrc`, `FLOYD.md`,
  `~/.config/floyd/`, `floyd.db`, `floyd.log`.
- Identity: `FLOYD_IDENTITY` env var > `agent_name` option > "Floyd".
  System-prompt overrides can be placed in `~/.config/floyd/prompts/`.
- One cassette-bound test (`TestCoderAgent`) is currently skipped pending a
  cassette re-record after the rebrand.

## Architecture

```
main.go                            CLI entry point (cobra via internal/cmd);
                                   swag OpenAPI annotations live here
internal/
  app/app.go                       Top-level wiring: DB, config, agents, LSP, MCP, events
  cmd/                             CLI commands (root, run, login, logout, models,
                                   projects, server, schema, stats, sessions)
  backend/                         Session/agent backend service layer shared by
                                   TUI clients and the headless server
  server/                          Server exposing the backend over a Unix socket
                                   (Windows named pipe); SSE events, multi-client
                                   workspaces, e2e tests
  client/                          Client that dials the server (proto over socket)
  proto/                           Wire protocol types: agents, sessions, messages,
                                   permissions, skills, tools, MCP, history
  workspace/                       Multi-client workspaces keyed by resolved --cwd
  herdr/                           Headless agent-driving client
  commands/                        User-invocable command registry (palette)
  config/
    config.go                      Config struct, context file paths, agent definitions
    load.go                        floydrc and floyd.json loading and validation
    provider.go                    Provider configuration and model resolution
  shellconfig/                     Bash-powered config format (floydrc builtins)
  agent/
    agent.go                       SessionAgent: runs LLM conversations per session
    coordinator.go                 Coordinator: manages named agents ("coder", "task")
    hooked_tool.go                 Decorator that runs PreToolUse hooks before tool execution
    prompts.go                     Loads Go-template system prompts
    templates/                     System prompt templates (coder.md.tpl, task.md.tpl, ...)
    tools/                         All built-in tools (bash, edit, view, grep, glob, ...)
      mcp/                         MCP client integration
  hooks/                           Hook engine: runs user shell commands on hook events
  session/                         Session CRUD backed by SQLite
  message/                         Message model and content types
  db/                              SQLite via sqlc, with migrations
    sql/                           Raw SQL queries (consumed by sqlc)
    migrations/                    Goose-style schema migrations
  lsp/                             LSP client manager, auto-discovery, on-demand startup
  ui/                              Bubble Tea v2 TUI (see internal/ui/AGENTS.md)
  permission/                      Tool permission checking and allow-lists
  skills/                          Agent Skills discovery and loading
  shell/                           Bash command execution (mvdan.cc/sh interpreter)
                                   with background job support
  oauth/                           OAuth flows (MCP servers, provider login)
  event/                           Telemetry (PostHog)
  pubsub/                          Internal pub/sub for cross-component messaging
  filetracker/                     Tracks files touched per session
  history/                         Prompt history
  update/                          Self-update checks
  (plus small utility packages: ansiext, csync, diff, diffdetect, discover,
  dns, env, filepathext, format, fsext, lock, log, projects, question,
  stringext, version, ...)
```

### Key Dependency Roles

- **`charm.land/fantasy`**: LLM provider abstraction layer. Handles protocol
  differences between Anthropic, OpenAI, Gemini, etc. Used in
  `internal/app` and `internal/agent`.
- **`charm.land/bubbletea/v2`**: TUI framework powering the interactive UI.
- **`charm.land/lipgloss/v2`**: Terminal styling.
- **`charm.land/glamour/v2`**: Markdown rendering in the terminal.
- **`charm.land/catwalk`**: Community model catalog fetched for provider and
  model metadata; also used for snapshot/golden-file testing.
- **`charm.land/x/vcr`**: HTTP record/replay ("cassette") testing for agent
  tests against real provider traffic.
- **`sqlc`**: Generates Go code from SQL queries in `internal/db/sql/`.
- **`mvdan.cc/sh`**: Embedded Bash interpreter used for the `bash` tool and
  the `floydrc` config format.

### Key Patterns

- **Client/server split**: the TUI is a client of a `backend` service; a
  headless `floyd serve` exposes the same backend over a Unix socket. Multiple
  clients pointed at the same `--cwd` join one workspace and share sessions,
  permission queue, LSP, and MCP state. Workspace flags like `--yolo` and
  `--debug` follow a first-wins rule.
- **Config is a Service**: accessed via `config.Service`, not global state.
- **Tools are self-documenting**: each tool has a `.go` implementation and a
  `.md` (or `.md.tpl`) description file in `internal/agent/tools/`.
- **System prompts are Go templates**: `internal/agent/templates/*.md.tpl`
  with runtime data injected.
- **Context files**: Floyd reads `AGENTS.md`, `FLOYD.md`, `CLAUDE.md`,
  `GEMINI.md` (and `.local` variants) from the working directory, plus global
  context from `~/.config/floyd/FLOYD.md` and `~/.config/AGENTS.md` by
  default. Project initialization writes `AGENTS.md` (configurable via
  `option initialize-as`).
- **Bash config format**: the primary config format is `floydrc` — a Bash
  script using builtins (`provider`, `model`, `mcp`, `lsp`, `permissions`,
  `hook`, `option`, ...) to define config. `floyd.json` is still supported
  but deprecated and receives no new features. Shell config files are
  discovered alongside JSON configs and deep-merged through the same
  pipeline. Builtins are registered via `shell.RegisterBuiltin` and gated by
  a `ConfigBuilder` on the context — they are no-ops during normal bash tool
  execution. See `internal/shellconfig/` and `docs/config/`.
- **Persistence**: SQLite + sqlc. All queries live in `internal/db/sql/`,
  generated code in `internal/db/`. Migrations in `internal/db/migrations/`.
  After editing SQL or migrations, run `task sqlc`.
- **Pub/sub**: `internal/pubsub` for decoupled communication between agent,
  UI, and services.
- **Hooks**: user-defined shell commands in `floydrc` (or `floyd.json`) that
  fire on hook events (currently `PreToolUse`). The engine
  (`internal/hooks/`) is independent of fantasy and agent — it takes inputs,
  runs commands, returns decisions. The `hookedTool` decorator in
  `internal/agent/hooked_tool.go` wraps tools at the coordinator level.
  Hooks run in parallel but compose in config order, and run before
  permission checks. Hooks are Claude Code-compatible. See `docs/hooks/`.
- **Skills**: Agent Skills open standard. Global skill paths include
  `~/.config/floyd/skills/`, `~/.config/agents/skills/`, `~/.agents/skills/`,
  `~/.claude/skills/` and `$CRUSH_SKILLS_DIR`; project paths include
  `.agents/skills`, `.floyd/skills`, `.claude/skills`, `.cursor/skills`.
  Repo-shipped agent guidance lives in `.agents/skills/`.

## Build/Test/Lint Commands

Use [go-task](https://taskfile.dev) (`task`) or raw `go` commands.

- **Build**: `task build` or `go build .`
- **Run**: `task run` (or `go run .`); `task dev` runs with pprof profiling
  on `localhost:6060`; `task run:onboarding` runs with fresh temp config dirs.
- **Test**: `task test` (= `go test -race -failfast ./...`). Single package:
  `go test ./internal/config`; single test:
  `go test ./internal/config -run TestName`.
- **Golden files**: `go test ./... -update` regenerates `.golden` snapshot
  files when TUI output changes intentionally. Update a specific package
  with `go test ./internal/ui/... -update`.
- **VCR cassettes**: `task test:record` (alias `record`) deletes and
  re-records `internal/agent/testdata` (requires real provider credentials;
  takes up to an hour). Do not run this casually.
- **Lint**: `task lint` / `task lint:fix` (golangci-lint v2 plus
  `task lint:log`, which enforces capitalized log messages via
  `scripts/check_log_capitalization.sh`).
- **Format**: `task fmt` (`gofumpt -w .`); `task fmt:html` runs prettier on
  the stats page assets.
- **Modernize**: `task modernize` (applies `modernize` analyzers).
- **sqlc**: `task sqlc` — regenerate DB code after editing
  `internal/db/sql/` or `internal/db/migrations/`.
- **Schema**: `task schema` — regenerate `schema.json` (the JSON schema for
  configuration).
- **Swagger**: `task swag` — regenerate the OpenAPI spec in
  `internal/swagger/` from annotations.
- **Hyper**: `task hyper` — `go generate` the embedded provider catalog in
  `internal/agent/hyper/provider.json`.
- **Deps**: `task deps` — update `charm.land/fantasy` and `charm.land/catwalk`.
- **Nix**: `flake.nix` provides a dev shell with go, gopls, golangci-lint,
  gofumpt, task, delve, gh, svu, and sqlc (`.envrc` uses `use flake`).

## Code Style Guidelines

- **Imports**: group stdlib, external, then internal packages; gofumpt
  formatting (stricter than gofmt) is enforced via golangci-lint.
- **Naming**: standard Go conventions — PascalCase for exported,
  camelCase for unexported.
- **Types**: prefer explicit types; use type aliases for clarity (e.g.,
  `type AgentName string`).
- **Error handling**: return errors explicitly; wrap with `fmt.Errorf`.
  (`errcheck` and a few other linters are intentionally disabled — see
  `.golangci.yml` for the curated list.)
- **Context**: pass `context.Context` as the first parameter for
  operations that can block or do IO.
- **Interfaces**: define interfaces in consuming packages; keep them small.
- **Constants**: typed constants with `iota` for enums, grouped in const
  blocks.
- **JSON tags**: snake_case field names.
- **File permissions**: octal notation (`0o755`, `0o644`).
- **Log messages**: must start with a capital letter ("Failed to save
  session", not "failed to save session"). Enforced by `task lint:log`.
- **ANSI strings**: never manipulate ANSI-coded strings at byte level; use
  `github.com/charmbracelet/x/ansi` helpers (`ansi.Cut`, `ansi.StringWidth`,
  `ansi.Strip`, `ansi.Truncate`).

## Testing Instructions

- Use testify's `require`, parallel tests with `t.Parallel()`, and
  `t.Setenv()` for environment variables.
- Always use `t.TempDir()` for temporary directories; no manual cleanup
  needed.
- Agent tests run against recorded provider traffic via `charm.land/x/vcr`
  cassettes in `internal/agent/testdata/`. Cassettes are generated files
  (see `.gitattributes`); re-record only with `task test:record` and only
  when the recorded flows legitimately changed.
- Golden/snapshot files (`.golden`, catwalk) are also generated; update with
  `go test ./... -update` only when output changes are intended.
- `TestCoderAgent` is currently skipped pending a cassette re-record after
  the rebrand; leave the skip in place unless you are re-recording.
- The server/backend packages have e2e and race tests
  (`internal/server/`, `internal/backend/`); run the full suite with
  `task test` (race-enabled) before finalizing changes to those areas.

## Formatting

- ALWAYS format any Go code you write.
  - First, try `gofumpt -w .` (or `task fmt`).
  - Fallbacks, in order: `goimports`, then `gofmt`.
- Comments that live on their own lines start with capital letters, end
  with periods, and wrap at 78 columns.

## Committing

- ALWAYS use semantic commits (`fix:`, `feat:`, `chore:`, `refactor:`,
  `docs:`, `sec:`, etc).
- Keep commits to one line unless additional context is truly necessary.
- Never push to `origin` — it points at the upstream Charm repository. This
  fork is private.

## Working on the TUI (UI)

Anytime you need to work on the TUI, read `internal/ui/AGENTS.md` before
starting work. It covers the hybrid Ultraviolet/Bubble Tea v2 rendering
pipeline, layout rectangles, state and focus rules, and golden-file testing.

## Styling System

The styling system lives in `internal/ui/styles/` and is organized into
three layers:

- **`quickstyle.go`**: The stable base theme builder. `quickStyle(opts)`
  constructs a `Styles` struct from `quickStyleOpts` — a palette of design
  tokens (primary, secondary, fgBase, bgBase, success, error, etc.).
  `quickStyle` must be fully token-driven: never hardcode specific
  `charmtone.*` colors here (except Chroma syntax highlighting, which is
  pending tokenization).
- **`themes.go`**: Defines concrete themes. Each theme function (e.g.
  `CharmtonePantera`) calls `quickStyle` with its palette, then applies
  theme-specific overrides as needed.
- **`styles.go`**: Defines the `Styles` struct — the shape of what
  `quickStyle` produces.

**Adding theme-specific overrides**: when a style genuinely needs a color
that doesn't fit the token model, keep `quickStyle` on the closest semantic
token and override only the differing colors in the theme function:

```go
func CharmtonePantera() Styles {
	s := quickStyle(quickStyleOpts{ /* palette */ })

	// Override only the colors that differ from the token defaults.
	s.Editor.PromptBangIconFocused = s.Editor.PromptBangIconFocused.
		Foreground(charmtone.Salt).
		Background(charmtone.Hazy)

	return s
}
```

**Adding a new theme**: add a function in `themes.go` that returns the
result of `quickStyle` with a `quickStyleOpts` palette (plus any needed
overrides), then wire it into `ThemeForProvider`.

## Security Considerations

- **Config is trusted code**: `floydrc` runs in a full shell, and any
  `$(...)` in `floyd.json` is expanded at load time. Never point Floyd at a
  directory whose config you haven't reviewed, and never `source` untrusted
  files from config.
- **Hooks run arbitrary shell commands** on hook events. Treat hook inputs
  and outputs as trusted config, not as sandboxing.
- **Tool permissions**: tools prompt for permission by default;
  `permissions allow/deny` and the `--yolo` flag (skips all permission
  prompts) must be handled carefully in code paths that affect them. Never
  weaken permission checks without an explicit reason recorded in the change.
- **API keys** arrive via environment variables (see the table in
  `README.md`) or provider config; never commit or log secrets. MCP configs
  may reference secrets through shell expansion (`$(op read ...)`).
- **License**: upstream code is FSL-1.1-MIT. This fork is internal-only;
  do not redistribute source or binaries outside the Legacy AI fleet.

## Further Reading

- `README.md` — user-facing feature and configuration documentation.
- `docs/config/` — the `floydrc` format, versioning, and legacy JSON.
- `docs/hooks/` — the hook protocol and examples (Claude Code-compatible).
- `internal/ui/AGENTS.md` — TUI-specific development rules.
- `.agents/skills/` — repo-shipped guidance for agents working on Floyd
  itself (builtin skills and shell builtins).
- `schema.json` — generated JSON schema for `floyd.json` (run `task schema`
  after config struct changes).
