# MegaFloyd advertised behavior repair

Source: owner's active goal and supplied `pasted-text-1.txt`, preserved privately
in `.tmp/tui-repair-20261031/owner-request.txt`. Existing claims in the supplied
audit are leads to reproduce, not proof of a cause or a completed repair.

## Required outcomes

Each unchecked row remains required. A source change or a registered command is
insufficient: acceptance requires the user's action and resulting visible state,
with real storage, tool execution or server routing wherever applicable.

| ID | Required behavior | Verified evidence |
|---|---|---|
| P1 | Palette Ctrl+Y invokes the advertised command regardless of highlight | Actual terminal palette-shortcuts-verified; Ctrl+Y toggles advertised mode from another selected row. |
| P2 | Category navigation works in both directions; hints reflect availability | Actual terminal forward Tab and reverse Shift+Tab; no-category regression. |
| P3 | Argument hints appear in the palette | Rendered palette/navigation screens and argument-hint regression. |
| P4 | Empty selections give useful feedback and preserve entered text | Actual terminal no-match warning retains exact typed filter. |
| P5 | Child menus return to the same parent, filter and selection; explicit Back key | Actual provider/theme/library/info child Back flows; async discovery and real event-loop regressions. |
| C1 | `/compact` uses the supplied instruction | Actual summary provider request includes RETAIN_EXACT_COMPACTION_INSTRUCTION; summary context regression. |
| C2 | `/model <name>` selects the intended model | Actual terminal selected reader-model; following real request used reader-model. |
| C3 | `/provider` browses providers and their models | Actual terminal provider -> model -> provider -> original palette flow. |
| C4 | `/undo` updates storage and displayed transcript | Actual terminal withdraws UNDO_VISIBLE_MARKER; real SQLite recovery record and visible-list regression. |
| C5 | `/title` immediately updates visible session title | Actual local and HTTP terminal title updates; atomic rename preserves newer recorded usage. |
| C6 | `/copy` reports actual clipboard delivery or useful failure | Actual /copy UI acknowledgement plus exact macOS pbpaste match; all original clipboard representations restored. |
| C7 | `/init` initializes the project without an unexpected session switch | Real /init UI dispatch through SessionAgent/native writes/SQLite in existing session; model failure and navigation races covered. |
| C8 | `/goal`, `/fork`, `/reload`, `/tasks` work in local and server modes | Actual local and HTTP terminal goals/fork/reload/tasks; workspace runtime integration tests. |
| L1 | TUI hook explanation and trace, and offline CLI inspection | Actual terminal explain/trace; actual offline CLI hooks; effective runtime tests. |
| L2 | Rewind restores tracked file state and conversation safely with protocol access | Actual terminal authorized write -> history -> rewind restores original bytes and retains quarantine; HTTP/concurrent recovery tests. |
| L3 | Users can inspect file versions | Actual terminal lists native file versions; version-content regressions. |
| L4 | Release notes are accessible and identify their actual version | Actual Release Notes panel identifies running build; static release dates retained. |
| L5 | `/todos` shows the current todo list | Actual Todos empty panel; current-session todo rendering. |
| L6 | `/agents` opens the agent library | Actual Agent Library panel; selecting entry retains draft and exposes editor regression. |
| L7 | `/skills` opens the skills library | Actual Skills Library panel; selecting entry retains draft and exposes editor regression. |
| L8 | TUI and CLI doctor check actual installation, configuration and environment health | Actual local/HTTP Doctor panel and CLI inspection. |
| L9 | Prompt templates can be selected, rendered and used from the TUI | Actual prompt list and rendered EXACT_TEMPLATE_INPUT in editor; existing draft preserved. |
| L10 | Codebase analyze/deps/search are usable from the TUI | Actual terminal analyze/deps/search and real local/HTTP frontend operation tests. |
| L11 | Session export and import are usable from the TUI | Actual terminal exports engine session and imports same ID; real local/HTTP storage tests. |
| L12 | Local telemetry is accessible with existing opt-in privacy preserved | Actual local telemetry collection/report; local-only shipping and redirect tests; privacy opt-in preserved. |
| L13 | `/memory` inspects the actual checkpoint | Actual memory checkpoint/empty-state display; shared workspace-host implementation. |
| L14 | Schema is discoverable in CLI help | Actual CLI help/schema checks. |
| S1 | Named swarm agents select actual distinct configurations, models and tool roles | Actual terminal and CLI writer/reader model results; real HTTP role enforcement and durable restart/fork tests. |
| S2 | Swarm progress, cancellation and results are visible and actionable | Actual terminal results and task list; actual HTTP completion/cancel plus source-workspace polling regression. |
| S3 | Swarm on/off/status/parallel controls persist for the correct session | Actual terminal swarm on/parallel status; durable local/HTTP controls and queued-child wakeup regressions. |
| Q1 | Question submission retains the draft until delivery acknowledgment | Actual local/HTTP terminal question answer retained until delivery; keyboard/service persistence regression. |
| Q2 | Failure/retry, duplicate and stale answers are handled without silent loss | Failure/retry/restart/duplicate submission integration and race tests. |
| Q3 | Delayed old notifications cannot dismiss a newer question; accepted answers remain visible | Actual full accepted answer remains visible; stale-batch/notification and protected-answer replay regressions. |
| F1 | Positional prompt and `-p/--print` execute the supplied prompt | Actual local CLI prompt and print checks. |
| F2 | `-c` continues; `--cwd` remains an explicit directory option | Actual local/server continue checks and explicit cwd isolation. |
| F3 | `-r/--resume` opens a picker or resumes an ID; web handoff remains supported | Actual terminal --resume picker and CLI full-ID resume; web compatibility tests. |
| F4 | Root `--model` applies to the real session | Actual configured reader-model request observed. |
| F5 | Text, JSON and stream-JSON output preserve final result and failure semantics | Actual local/server incremental streaming, JSON and text; SIGINT yields nonzero and final error metadata. |
| F6 | `--add-dir`, `--permission-mode`, `--allowedTools` reach actual tool boundaries | Actual local/server permission denial, allowlist, plan and additional-directory checks; redirected-write bypass repaired. |
| F7 | Skip-permissions alias preserves protected deletion confirmations | Actual local/server sentinel preserved with skip-permissions alias; protected deletion regressions. |
| F8 | MCP, config, update and setup-token commands perform their advertised jobs | Real stdio MCP, persisted config, synthetic device-login flow and authenticated GitHub release check; redaction negative tests. |
| F9 | Empty stats is a successful empty state; identity/version flags are consistent | Actual empty stats exit zero and identity/version checks. |
| E1 | `/context`, `/cost`, `/vim`, `/terminal-setup`, `/output-style`, `/privacy-settings` perform their named jobs | 63-step actual terminal panels/editor run; persisted profiles/privacy, actual Vim edits/undo, rendering and runtime analytics tests. |
| V1 | Actual rendered terminal flows and real client/server boundaries pass | Real PTY local/server terminal captures, deterministic loopback model, SQLite, HTTP, native tools and macOS clipboard. |
| V2 | Affected tests, full Go tests, static checks and builds pass | Full race pass; final affected race pass; fresh full standard suite pass (82 tested packages); vet/build pass. |
| V3 | Approved instructions, user data and deletion protections remain intact | Policy fingerprint and generated copies unchanged; baseline backup hashes intact; protection suite pass; quarantined recovery. |

## Current execution

- Source baseline: `245317e5dc78e60aed89fa8f4b4032a5207c8389`; owner merge
  `c9e3937b579aa6fb2155ad495cf3ab347bc87b5b` has the same source tree.
- Sixteen pre-existing modified/untracked files were copied with hashes into
  `.tmp/tui-repair-20261031/baseline`; no existing edits were discarded.
- Primary integration owner: root. Component owners: CLI, question delivery,
  and swarm/server operations. Shared file edits are coordinated.
- Production application replacement and installation remain excluded.
- The canonical project record is now `docs/MEGAFLOYD_SSOT.md`. The old
  `SSOT.md` path remains a working relative symlink for its existing readers.
- The supplied audit's claimed root causes and prior test results are being
  rechecked; they are not silently treated as current proof.

## Evidence location

The consolidated result and verification boundaries are in
[the repair evidence](../evidence/tui-repair-20261031.md). Detailed local fixtures,
terminal captures, original failures, test exit records, source manifests and
baseline backups remain under `.tmp/tui-repair-20261031/`; private clipboard
backups stay ignored. No production installation was performed.
