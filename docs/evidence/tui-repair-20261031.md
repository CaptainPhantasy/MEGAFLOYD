# MegaFloyd advertised behavior repair — verified candidate

Observed on 2026-10-31 in `/Volumes/SanDisk1Tb/MEGAFLOYD`, macOS account `floyd`.
Working branch: `codex/megafloyd-modern-hooks`; unchanged history at
`245317e5dc78e60aed89fa8f4b4032a5207c8389`. This report covers the owner's TUI/CLI
repair request and supplied command audit. It does not represent a production
installation or a new GitHub publication.

The complete acceptance mapping is in [the requirement matrix](../plans/tui-repair-20261031.md).
All listed requirements have current executable evidence at the appropriate UI,
storage, process, or HTTP boundary. Original failed runs remain preserved.

## Corrections

- Palette shortcuts now select their advertised command, category navigation works
  both ways, argument hints are visible, and no-match input is retained. Parent
  menus retain their state. Exact command names cannot be redirected by fuzzy
  matches. Background discovery preserves in-progress input. Framework sequence,
  batch and quit messages reach Bubble Tea's actual event loop.
- Named model selection activates the selected model. Provider browsing reaches
  its models. Compact instructions reach the real summary request. Undo updates
  both durable history and visible messages. Titles update atomically without
  overwriting newer usage. Initialization uses the existing session and only
  reports initialization after real tools succeed. Deferred sends and session
  loads retain their workspace/session identity and reject obsolete results.
- Questions retain drafts until durable acknowledgement, support safe retries,
  and reject stale or conflicting replies. Protected deletion confirmations are
  never reused as ordinary question answers. Completed answers remain visible.
- File history and rewind use verified native checkpoints. Rewind refuses unsafe,
  changed or untracked effects, preserves displaced data in quarantine, and
  recovers independent files after partial failure. Local and HTTP paths share
  these protections.
- Goal, fork, reload, tasks, plan, summaries and undo route through local or server
  workspaces. Named swarm agents use their configured models and restricted tool
  roles. Progress, cancellation and results are real. Restrictions survive
  restart and forks; child agents inherit plan mode. Navigation cannot move task
  polling into a different workspace.
- Hooks, release notes, todos, agent/skill libraries, doctor, prompt templates,
  codebase operations, session import/export, telemetry, memory and schema are
  reachable. Library/template insertion preserves existing drafts.
- CLI prompt/print/continue/resume/model/output/permission options are functional.
  MCP administration uses a real connection, configuration edits persist, and
  setup-token uses the actual device-auth flow. Release checking uses the correct
  repository and package verification. Streaming completion matches both session
  and run; cancellation returns an error result and nonzero exit.
- Context/cost show stored session facts. Vim keys edit the actual prompt, terminal
  profiles change send/newline behavior, output style changes rendering, and
  privacy settings persist with an immediate analytics gate.
- Verification exposed and repaired a plan-mode shell-redirection bypass and a
  configuration-output leak through credential-bearing URLs/arguments. The
  protected deletion guard remains effective under ordinary permission shortcuts.

## Verification

| Check | Observed result |
|---|---|
| Full Go suite, `go test -p 1 ./... -count=1 -timeout=5m` | Exit 0; 82 tested packages passed; 36 packages have no tests |
| Full race suite, `go test -race -p 1 ./... -count=1 -timeout=5m` | Exit 0; 82 tested packages passed |
| Final changed CLI/model/dialog packages with race detection | Exit 0 after the last review corrections |
| `go vet ./...` | Exit 0 |
| Build with `CGO_ENABLED=0 GOEXPERIMENT=greenteagc` | Exit 0 |
| Final focused palette, core commands, frontend/recovery, workspace, CLI, preferences and protection checks | All exit 0 |
| Actual local CLI matrix | 31/31 accepted results |
| Actual server CLI matrix | 12/12 accepted results |
| Admin checks | 4/4 accepted results |
| Credential-redaction executable checks | Both commands exit 0; fixture secrets absent; original config unchanged |
| Actual macOS `/copy` | Visible delivery acknowledgement and exact `pbpaste` bytes; original clipboard restored completely |
| Instruction copies and embedded prompt pack | Match approved source |
| Final source-manifest comparison | No drift across 1,828 entries |
| Final diff/scope/security inspection | No remaining actionable finding identified |

Terminal evidence includes a 63-step command/editor walkthrough; local and HTTP
question delivery; a 22-step actual server TUI flow; provider/model/back navigation;
Ctrl+Y, category and no-match behavior; a 26-step final-candidate runtime flow;
import/libraries/templates; and authorized native edit/history/rewind. The latter
restored `ORIGINAL_RECOVERY_CONTENT` and kept the displaced bytes in quarantine.
The actual summary request contained the exact supplied compaction instruction.

Provider, MCP and auth fixtures were deterministic and isolated. They exercise the
real application, tools, SQLite, HTTP, terminal and process paths without paid
model calls or user-data changes. This does not establish availability or output
quality of every external provider. The real authenticated GitHub release check
found no published release; no update package was installed. Test packages with
no tests are not counted as tested packages. The full race run was followed by
race checks for the later changed packages and a fresh full standard suite.

## Candidate and preservation

Runnable candidate: `.tmp/tui-repair-20261031/candidate/megafloyd`.

SHA-256: `465678ffb38d763a82260fd72148a93c33805d5c82cc67a49d2377112942f550`.

Approved `MegaFloyd.md` remains exactly
`5bf0bb4a37990bfd1c7f7cc22c73e1ec2bfbfddac0d95952bae49652aa40945b`.
All 16 pre-existing work backups match their recorded hashes. Existing SSOT
content is retained in `docs/MEGAFLOYD_SSOT.md`; `SSOT.md` is its compatibility
symlink. User files and clipboard content were preserved. No production install,
merge, force push, or permanent deletion of user data occurred. Existing working
tree edits were integrated without resetting history. New repairs remain local
and uncommitted; this report does not claim publication or a clean working tree.

Detailed execution artifacts are retained under `.tmp/tui-repair-20261031/`:
`swarm/verification-final.json`, full test logs and source manifest;
`terminal/` scripts/screens; `cli/REPORT.md` and `cli/SECURITY-REVIEW.md`;
`question/agent-continuation.md`; regression receipts and baseline manifest.
Private clipboard backups stay ignored and must never be included in a commit.
