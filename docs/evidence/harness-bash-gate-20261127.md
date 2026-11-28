# Bash deletion gate repair, 2026-11-27

Owner outcome (reported, quoted by the repair brief `/tmp/harness-repair-brief.md`):
"repair your own harness by loosening the restrictions to match the user
selected modes".

Scope: source under `floyd-go`, on top of `main` `ff0dd0c`. Nothing was
installed, released, committed or restarted, so the installed `2026.11.22`
still carries the old gate.

Labels: `checked` = run or read in this session. `reported` = stated by another
party. `inferred` = concluded from evidence. `unknown` = not established.

## Diagnosis

The installed release `2026.11.22` is built from `010a141`, and
`git diff --stat 010a141 HEAD -- floyd-go/internal/deletionguard floyd-go/internal/agent/deletion_guard.go floyd-go/internal/approvalauthority`
is empty (checked). A probe on `main` therefore reproduces the live classifier.

The session's own tool calls were read from its workspace database
(`8cf7d1c2…`, 15:53–15:58 on 2026-11-27, checked). Four rules refused them:

| Command the yolo session sent | Why the installed gate refused it | Change |
|---|---|---|
| `git -C /tmp/gaptest-repo status --short`, `git -C /tmp/gaptest-repo add net.py` | Any option before the subcommand failed closed: "git global options can change the repository or command dispatch" | Options before the subcommand are stepped over and the subcommand decides. `git rm` behind options, and any option not recognized, still fail closed. |
| `cd /tmp/gaptest-repo && echo test > b.txt` | Any `cd` made later relative targets unprovable | A literal `cd` to an existing directory on the left of `&&` proves the directory. This edit was already in the working tree from 2026-11-23 and is kept; it now has a repro. |
| `rm /tmp/gap_verdicts.py`, `rm -r /tmp/gaptest-repo` | Every `rm` was permanent deletion | An unforced `rm` with literal operands is an ordinary removal. `-f`, `--force`, an unrecognized flag or a nonliteral operand is still permanent deletion. |
| `mkdir -p /tmp/quarantine-gap-fixtures && mv <4 items> /tmp/quarantine-gap-fixtures/` | The destination did not exist while the command was read, and several sources need a verified directory | A directory the command creates is treated as present and empty for the rest of the command. Replacing an item already inside it is still permanent deletion. |

The hard block behind all four (checked): on macOS `authorityTransportSupported()`
always returns `ErrTransportUnavailable` (`internal/approvalauthority/peer_darwin.go:105`).
The approval service cannot run, so no confirmation can be read or recorded,
and every gated command failed closed with "start the administrator-owned
approval service", which nothing the owner can do on this platform would fix.

Runtime state (checked): no `deletions` directory and no `record.json` exist
under `~/.megafloyd`, so nothing stale needed clearing. The live process is
`megafloyd -y` (`ps -p 76205`). `/Users/floyd/.megafloyd/config/floyd.json`
holds only an `mcp` key; the permission mode is not stored there.

The classifier has three consumers (checked by grep over non-test code): the
Bash and parallel-Bash gate (`internal/agent/deletion_guard.go`), the backend's
remote shell route (`internal/backend/agent.go:371`), and the coordinator that
builds the guard. The remote route has no confirmation channel and rejects any
destructive command outright; it never used the approval service, so only the
classification changes reach it.

## Requirements

| R | Requirement | Check | Status | Evidence |
|---|---|---|---|---|
| R1 | Under the selected mode, the commands the session could not run reach the tool | `TestYoloSessionCommandsRunThroughRealBashTool`: the session's commands replayed through the real Bash tool in the mode `-y` selects | PASS | both variants ran, 0 failures |
| R2 | Where the approval transport is unsupported, the gate steps aside with a one-line note | `TestDestructiveCommandFollowsPermissionModeWhenAuthorityUnavailable`, `TestDefaultRuntimeGateStepsAsideWhereNoServiceCanRun`, `TestUnavailableApprovalAuthorityNoteStaysOnOneLine` | PASS | command ran; one line; none for harmless calls |
| R3 | A working or merely unreachable approval service changes nothing | existing two-confirmation tests, `TestDestructiveCommandStaysBlockedWhenAuthorityUnreachable`, `TestUnsupportedTransportIsDistinctFromAnUnreachableService` | PASS | unreachable service still fails closed |
| R4 | `git` options before a read-only subcommand are not deletion; destructive git behind options still is | repro case, `TestGitOptionsBeforeSubcommand`, `TestLoosenedClassificationsKeepTheirBoundaries` | PASS | |
| R5 | A relative redirect after `cd … &&` to a new file is not deletion; truncating an existing file still is | repro case, boundary case "cd then truncate an existing file" | PASS | |
| R6 | Unforced `rm` and `rm -r` are not deletion; forced, unrecognized and nonliteral forms still are | repro cases, `TestUnforcedRemovalIsNotDestructive`, `TestPotentialDispatchStaysDestructive`, boundary cases | PASS | |
| R7 | A move into a directory the same command creates is quarantine; replacing an existing item stays deletion | repro case, `TestMoveOverExistingQuarantineCopyStaysDestructive`, `TestCreatedDirectoryIsUsableForTheRestOfTheCommand` | PASS | |
| R8 | Repro cases exist for every blocked example and fail without the fix | `classify_repro_test.go` copied into a pristine `HEAD` copy and run | PASS | every blocked example fails there with its diagnosed message |
| R9 | `go build ./...` and `go test ./internal/deletionguard/... ./internal/approvalauthority/...` are green | commands below | PASS | |
| R10 | Behavioral rule documents, the steward suite, provider/model settings and credentials are untouched | `git status --short` before and after | PASS | only the files listed below changed |
| R11 | Stale deletion-session record cleared if part of the bug | `find ~/.megafloyd -maxdepth 6` | NA (trigger false) | no such record exists |
| R12 | Fix reaches the live session | | NOT_RUN | needs a release build and a restart, by instruction not done here |

## Two deliberate reversals, for the owner to confirm

Both follow the brief's explicit wording. Both loosen rules the suite had
pinned, and SSOT §5.1 reads "Never weaken permission checks". Neither takes
effect until a release is built.

1. **Plain `rm` and `rm -r` are no longer gated in any permission mode, on
   every platform.** The force flag is the boundary. Before, they needed two
   confirmations wherever the approval service could run; now only `rm -f`,
   `rm -rf` and the other removal commands do. The remote shell route, which
   has no confirmation channel, used to refuse every `rm` and now runs the
   unforced ones.
2. **Where the approval service cannot run, the gate steps aside in every
   mode**, manual included. A forced removal then meets only the ordinary
   permission prompt. Before, it was refused with no way forward.

To narrow either one: for the first, restore the two lines in
`internal/deletionguard/classify.go` that return early for an unforced `rm`,
and restore the test fixtures from git; for the second, delete the
`UnsupportedTransport` branch in `internal/agent/deletion_guard.go`.

## Note destination

The brief asks for a one-line stderr note. That is what happens whenever
stderr is a capture stream (checked: a throwaway test with production defaults
put exactly one line on the test binary's stderr, for the one forced removal
and not for `echo hello`). While the terminal interface is active, process
stderr is the screen itself, and the pinned renderer repaints only lines it
knows changed (`ultraviolet` `terminal_renderer.go:1177-1225`, checked), so a
line written there would stay on screen. In that case the same note goes to
the application log (`slog.Warn`, "Deletion check skipped"). This is a
deviation from the letter of the brief in the owner's own terminal session and
is called out in the final report.

## Test evidence

All run from `floyd-go` on the final bytes (checked):

- `go build ./...` exit 0.
- `go test ./internal/deletionguard/... ./internal/approvalauthority/...`: both `ok`.
- `go test -count=1 ./internal/agent/...`: nine packages, all `ok`.
- `go test -count=1 ./...`: 119 packages `ok`, 0 fail, including every package the CI workflow lists.
- `go vet ./...` exit 0.
- `go test -race` on the two guard packages and the agent deletion tests: `ok`.
- `gofumpt -l` on every touched file: nothing listed.
- `golangci-lint`: unknown, not installed here. CI runs it.

Source identity (SHA-256, first 16 hex): `classify.go` `f36dcddba0ee3101`,
`guard.go` `8f7d955d1b03f3ab`, `ledger.go` `0a9a0d00b9d514bf`,
`deletionguard.go` `cb8ac5fd50a1f056`, `agent/deletion_guard.go` `c729ffab631ca67f`.

Test fixtures: 247 unforced `rm` commands that stood for "a deletion" in
existing tests were changed to `rm -f`, by one script whose every change was
reviewed (184 in `internal/deletionguard`, 61 in `internal/agent`, 2 in
`internal/backend`), plus 2 by hand. Fixing only the failures would have left
assertions such as `require.True(Check("rm b").Allowed)` passing without
testing anything.

## Files

Source: `internal/deletionguard/classify.go`, `guard.go`, `ledger.go`,
`deletionguard.go`; `internal/agent/deletion_guard.go`.

Tests: `internal/deletionguard/classify_repro_test.go`,
`ledger_security_test.go`, `deletionguard_test.go` and the migrated fixtures;
`internal/agent/deletion_background_authority_unavailable_test.go`,
`deletion_guard_test.go`, `deletion_authority_transport_denial_test.go`
(untracked) and the migrated fixtures; `internal/backend/shell_deletion_guard_test.go`.

Not touched: `internal/config/provider.go`, `curated_models.go`,
`curated_models_test.go` and `classify_directory_proof_test.go` beyond three
fixture lines; these were already uncommitted for other work.

## Backups

Every edited file that was not cleanly tracked was copied first, hashes
verified, with a manifest giving the restore command:
`.quarantine/harness-fix-bak-20261127T213551Z/MANIFEST.txt`.

Evidence invalidators: any change to the five source files above, a new
release, or a different platform.
