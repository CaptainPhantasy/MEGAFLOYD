# Command surface: advertised-but-broken and latent features

Audit date: 2026-10-31. Host: Mac mini, `/Volumes/SanDisk1Tb/MEGAFLOYD`.
Evidence: source read + rendered palette in a test harness + live CLI probes of the built binary.
Existing suites (`internal/ui/dialog`, `internal/commands`, `internal/ui/model`) pass, so none of this is covered by tests.

## 1. Misleadingly advertised

Each row claims a behavior in help text, description or keybinding hint that the binary does not deliver.

| # | Surface | Claim | Actual | Evidence |
|---|---|---|---|---|
| 1 | Palette, `/yolo` row | `ctrl+y` shortcut | `ctrl+y` confirms the highlighted row; hint is dead unless `/yolo` is already selected | `dialog/commands.go:107` (Select = enter+ctrl+y), scan only in `default` at `:247`; measured `ActionRunBuiltin{Name:"permission"}` |
| 2 | Palette footer | `tab switch selection` | Inert when no user commands or MCP prompts exist | guard `commands.go:237`; `ShortHelp` `:364` unconditional; measured `nil` action |
| 3 | Palette rows | `ArgumentHint` for 12 commands | Never rendered anywhere | `commands/builtin.go:33-58`; `commands.go:500` drops it; measured `/plan` without `status\|clear\|cancel` |
| 4 | `/compact [instruction]` | Accepts an instruction | Argument silently discarded | `builtin.go:327` never reads `args` |
| 5 | `/model <name>` | — | Name ignored; only the picker opens | `builtin.go:277` |
| 6 | `/provider` | "Browse configured providers and models" | Byte-identical behavior to `/model`; no provider view exists | `builtin.go:286` |
| 7 | `/undo` | "Withdraw the last prompt" | Messages deleted in the store; transcript unchanged | `builtin.go:1180-1228`; `setSessionMessages` only on `loadSessionMsg` |
| 8 | `/title <name>` | "Show or set the session title" | Saved copy; header keeps old title until reload | `builtin.go:752-760`, `sidebar.go:79`, `ui.go:5552` |
| 9 | `/copy` | "Copy the last assistant message" | Reports success unconditionally; `WriteText` has no error return | `clipboard/clipboard.go:35` |
| 10 | `/init` | "Analyze the project and generate AGENTS.md" | Also creates a new session and switches to it | `onboarding.go:54` → `ui.go:4523` |
| 11 | `/goal`, `/fork`, `/reload`, `/tasks` | Plain descriptions | Local-workspace only; emit a warning in client/server mode | `builtin.go:1104`, `:1158`, `:1497`, `:597` |
| 12 | CLI `hooks explain\|trace` | "without starting a server or agent" | Dials a unix socket, exits 1 with no live server | verified: `dial unix …/floyd-*.sock` |
| 13 | Palette, Enter | "confirm" | Silent no-op when the filter matches nothing | measured: 0 items, `nil` action |
| 14 | CLI `stats` | "Show usage statistics" | Exits 1 `No data available: no sessions found in database.` — reads as failure, not empty state | verified |

## 2. Latent: engine exists, no entry or exit point

Each row is built and reachable from code, but a user has no way in (or no way out) without new wiring.

| # | Capability | What already exists | Missing entry/exit |
|---|---|---|---|
| 1 | Hook inspection | `internal/hooks` (event registry, `Runner.Preview`), `internal/hookstore` (queries by session/tool-call/event/job), agent tools `hook_explain`/`hook_trace` | No TUI command; CLI entry exists but is broken (see 1.12) |
| 2 | Rewind | Per-turn file snapshots with before/after content and versions: `history.Turn`, `TurnChange`, `CaptureBefore`, `TurnContent` | No restore write, no message truncation, no protocol exposure, no command |
| 3 | File version history | `history.Service`: `CreateVersion`, `ListBySession`, `GetByPathAndSession` | Nothing surfaces versions to the user |
| 4 | Release notes | `version.DefaultChangelog()` with structured entries | No UI; content stale (entries dated 2026-02-22) |
| 5 | Todo list | `session.Todo{Content,Status,ActiveForm}`; pill rendering | No `/todos`; only a collapse toggle |
| 6 | Agent Library | `dialog.NewAgentLibrary(config.GlobalAgentsDirs())`, palette entry | No `/agents` slash command |
| 7 | Skills Library | `dialog` + palette entry | No `/skills` slash command |
| 8 | Doctor report | `runhealth.Load/Enabled`, config audit, `cmd/doctor.go` readers | CLI only; no TUI command, and the readers live in package `cmd` |
| 9 | Prompt templates | `floyd prompt list` (7 templates), render, policy | CLI only |
| 10 | Codebase analysis | `codebase analyze\|deps\|search` | CLI only |
| 11 | Session export/import | `session export`, `session import` | TUI has `/export-md` only; no import |
| 12 | Local telemetry log | `telemetry collect\|report\|ship`, machine-local | CLI only |
| 13 | Memory checkpoint | `internal/megafloyd/memory` writes/reads `.supercache` (cwd, session, git state) | No `/memory` to inspect or open it |
| 14 | `schema` command | Registered on the root command | Hidden from help entirely |
| 15 | Swarm mode | `agent.RuntimeSwarm`/`SetRuntimeSwarm` on session metadata; `GET/POST /v1/workspaces/{id}/sessions/{sid}/swarm`; gates subagent parallelism in `acquireRuntimeTask` | No TUI or CLI entry. `/swarm <text>` only fanned a task out to every agent, so `/swarm on` dispatched the literal word "on". **Fixed 2026-10-31:** `/swarm on\|off\|status\|parallel N` (`/swarm run <task>` keeps the fan-out), with a `swarm` status pill and per-session refresh |

## 3. Menu traversal requirement

**Requirement:** a menu must be traversable forwards *and* backwards as a first-class path. Forward-only movement that ends in `esc` is not acceptable.

Where this fails today:

- Every forward move out of the palette destroys the palette. `ActionOpenDialog` and `ActionRunCustomCommand` call `CloseDialog(CommandsID)` / `CloseFrontDialog()` before opening the child (`ui.go:2153`, `:2346`), so the filter, selection and parent are gone.
- The dialog layer has no parent stack, so `esc` from a child does not go back — it lands in the editor.
- The footer advertises only the forward key (`tab switch selection`) and never the reverse.
- Arrow navigation itself is bound in the list body (`up`/`down` in `theme.go:78`, `models.go:139`, `sessions.go:112`, `plugins_library.go:122`, `skills_library.go:119`, `agent_library.go:120`, `notifications.go:107`), so the gap is the parent/child path, not the list body.
