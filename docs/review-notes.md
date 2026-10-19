# Review notes — audit remediation

Review date: 2026-10-19. Base commit: `1d15945` on `fix/house`.
These are the known caveats for anyone reviewing the waves 2–4 remediation.
Each item names where to look; none of them is a pass claim.

## 1. 137-file gofumpt churn

Wave 2 ran `gofumpt` across the tree and reformatted 137 chassis files that
the wave did not otherwise change (`SSOT.md:194`). Review semantic changes
per package; do not read the reformat as behavior change. On this worktree
`gofumpt -l .` exited 0 with no output.

## 2. Two command sources, only one embedded

- Project copy: `floyd-go/.floyd/commands/*.md`. This is also the default
  `<DataDirectory>/commands` path, because the default data directory is
  `.floyd`.
- Shipped copy: `internal/megafloyd/skills/data/*.md`, embedded with
  `//go:embed data/*.md` (`internal/megafloyd/skills/install.go:19`).

At startup `InstallCommands` writes the embedded files into
`<DataDirectory>/commands` and rewrites any file whose bytes differ
(`internal/app/app.go:175`, `internal/megafloyd/skills/install.go:148`).
Consequence: editing only `.floyd/commands` does not change what a fresh
binary installs, and starting the binary in this repo overwrites the local
copies with the embedded ones. This lane rewrote the seven job command files
in `.floyd/commands`; the mirror under `internal/megafloyd/skills/data/` was
not touched because it belongs to the skills-install lane. Copy the seven
files there before release and re-run
`go test ./internal/megafloyd/skills`.

The loader names project files `project:<file>`
(`internal/commands/commands.go:221`). The seven files are selected from the
palette; typing `/contract` is not a builtin and falls through as a normal
prompt (`internal/ui/model/builtin.go:223`). Their usage lines keep the
approved `/contract` naming; registering the seven as typed triggers is
outside this lane.

## 3. skills.json registry is stale

In `.floyd/commands/skills.json`, the entries for `handoff`, `contract`,
`prove`, `recover`, `drift`, `replay`, and `why` still describe pre-tool
workflows (`transcript_export`, `session fork`, `.harness/contracts/`,
`.harness/proofs/`, "deep ... plumbing pending"). The shipped tools are
`contract`, `prove`, `drift`, `why`, `recover`, `handoff`, `replay`
(`internal/agent/tools/jobs_register.go:20-26`). The loader and tests do not
read the `wraps` or `note` fields, so this stale text did not fail a gate.

## 4. Job-tool test coverage

`internal/agent/tools/jobs_tools_test.go` covers completion refusal, artifact
invalidation, `UNKNOWN` from `why`, out-of-scope drift, replay/handoff/recover,
and tool registration. It does not assert the text of the seven markdown
command files; those are prompt templates and are not compiled.

## 5. Admin auth is not verified

Routes were checked against `server /platform.js`; the auth key model is open
and needs the owner key. See the OPEN line at the end of `progress.md`.

## 6. Cloud and packaging gaps

- Codespaces push is blocked on owner authorization.
- Linux installer paths were not executed. The installer ran on macOS with a
  temporary prefix; qemu user-mode binaries were absent, so the Linux
  binaries were built but not run.
- The `/desktop` target decision is open.
- No live provider session was exercised; the prompt-kernel position is
  covered by unit tests only.

## 7. Cassette skip

`TestCoderAgent` remains skipped pending a cassette re-record
(`floyd-go/AGENTS.md:41` and `:229`). A green suite is not coverage of that
path.

## 8. Audit numbering mismatch

`FIX-PLAN.md` maps audit item 13 to the output-style engine and item 15 to
web/remote (`FIX-PLAN.md:15` and `:24`). The lane brief maps 13 and 15 to the
job command files and to the wave status plus review notes. This file follows
the lane brief; confirm the audit ledger before closing either number.
