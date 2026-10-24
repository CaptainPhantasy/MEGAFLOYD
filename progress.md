<!-- owner-final-release-hold-20261024:start -->
**Owner release hold — recorded 20261024T231641Z (UTC).**

Final build, package assembly, publication, installation, and runtime activation are held. The owner requires an advance alert before final build/packaging. Resume only after the coordinating root agent confirms the required advance notice was given, the artwork is ready, and this hold is lifted. Source edits, intermediate compilation, and focused read-only checks remain allowed.

The current installed and last-verified running release is `megafloyd-20261024T230715Z` (version `2026.10.24`). Preserve its app, session history, credentials, and current DeepSeek V4.1 Flash settings (384,000 maximum output tokens, High reasoning). Do not quit/restart the app or bootstrap the staged login job during this hold.

Candidate `megafloyd-20261024T231400Z` has intermediate native/main compilation outputs only. It has not been assembled, published, installed, or activated. Concurrent wordmark/branding work must be coordinated into the final source state before any authorized final package. Existing worker notes and historical verification entries below remain intact; their earlier completion statements do not lift this later hold.
<!-- owner-final-release-hold-20261024:end -->

# progress.md — MEGAFLOYD completion session log

## 2026-10-16 — session 1

### Phase 1 — complete: recon

- Read `README.md`, `PLAN.md`, `docs/MERGE-STATUS-20261016.md`, `docs/prompt-pack/MANIFEST.md`.
- Git: single commit `e33af3f` (initial scaffold); untracked = `docs/MERGE-STATUS-20261016.md`,
  `docs/prompt-pack/MANIFEST.md`, `scripts/`. No `.planning/`.
- Ran `node scripts/verify-prompt-pack.mjs` → **PASS** (derived + unversioned copies match).
- Measured toolchain: node v26.9.0 ✓, pnpm 10.25.0 (pinned 10.33.0), go go1.25.7 (go.mod 1.26.6).
- `floyd-code/node_modules` and `apps/floyd-code/dist` are now present (they were absent when
  MERGE-STATUS F5 was written).
- Verified all 11 corrected plug-point paths exist under `floyd-code/packages/agent-core-v2/src`.
- Confirmed F1: source repo `floyd-v6/src` still carries the PostHog key; merged copy remediated.

### Planning files created (this repo, tracked)

- `task_plan.md`, `findings.md`, `progress.md` at repo root, so memory travels with the project.

### Files written this session

- `/Users/floyd/MEGAFLOYD/task_plan.md`
- `/Users/floyd/MEGAFLOYD/findings.md`
- `/Users/floyd/MEGAFLOYD/progress.md` (this file)

### Phase 2–7 — complete: closure tranche

- **P1 prompt pack** closed: canonical = `docs/prompt-pack/templates/`; derived Go copy documented;
  `MANIFEST.md` present with the real `PromptDat` interface; guard PASS.
- **`PLAN.md`** corrected: plug-point table rewritten to the 11 verified paths; P1 variable list
  replaced with a pointer to `MANIFEST.md` §2 (the old CWD/ENV/GIT_STATUS/… names do not exist);
  stale `/Users/floyd/floyd-merge/prompt-pack/` framing replaced with the in-repo canonical pack.
- **`README.md`**: added a Prerequisites table (node/pnpm/go measured vs required) + `GOTOOLCHAIN`
  note + drift-guard usage; Status section now names P0/P1 done and links the docs.
- **`docs/DECISIONS.md`** created: D-1 company-platform = OUT of the ship build (reasoned);
  D-2 source-repo PostHog key recorded (owner-owned); D-3 prompt-pack single-source-of-truth decided.

### Phase 8 — gates green, committed

| Gate | Command | Result |
|---|---|---|
| Drift guard | `node scripts/verify-prompt-pack.mjs` | PASS |
| Floyd Code | `node floyd-code/apps/floyd-code/dist/main.mjs --version` | 2.0.2 |
| Go build | `go build ./...` | exit 0 |
| Go test | `go test ./internal/shell/...` | ok 4.890s |
| Go vet | `go vet ./internal/agent/...` | clean |

P2–P5 remain unstarted (roadmap in `task_plan.md`), not claimed done.

## 2026-10-16 — session 2 (build relocation to /Volumes/SanDisk1Tb/MEGAFLOYD)

### Trigger

Owner relocated the build to a fresh clone and ran `pnpm -r run build`; it failed repeatedly on
files that exist in local copies but were never committed.

### Diagnosis — one defect class, six instances (see `findings.md` §H)

Over-broad unanchored `.gitignore` rules hide real source at any depth.

- Fixed root-file instances 1–4 with a trailing negation block (`f8c4541`): `build/`, `logs/`,
  `credentials`, `.floyd/`. Verified with `git check-ignore` + negative controls.
- Committed + pushed from `/Users/floyd/MEGAFLOYD` (fast-forwarded to `d62c595` first).
- Sweep proof: `comm` of source-vs-mega tracked files = 525 = 524 `dist-web` + 1 `klient/Dockerfile`.

### Then — third blocker, still the same class

`check-web-assets.mjs` aborts: committed `apps/floyd-code/dist-web/` missing. Open → Track A A3.

### Planning files

- Rewrote `task_plan.md` as the master plan (Tracks A–D, gates, operating rules).
- Extended `findings.md` (§H inventory, §I unverified infra leads, §J toolchain).
- Added `docs/DECISIONS.md` D-4 (ignore-rule policy + guard) and D-5 (dist-web restore policy).

## 2026-10-16 — session 3 (build proof reproduced; Track A closed)

### Gate — fresh-copy build proof: PASSED

| Step | Command | Result |
|---|---|---|
| Install | `pnpm install --frozen-lockfile` in `floyd-code/` | exit 0; corepack used the pinned pnpm 10.33.0 |
| Build | `pnpm -r run build` (root script `build`; matches the source README's `pnpm build`) | exit 0; 23 packages reported Done |
| App version | `node apps/floyd-code/dist/main.mjs --version` | `2.0.2`, exit 0 |
| Web assets | `node apps/floyd-code/scripts/check-web-assets.mjs` | "Web assets OK … 525 entries" |

Toolchain measured: node v26.9.0, pnpm 10.25.0 on PATH (pinned 10.33.0 resolved by corepack),
go 1.25.7 against a `go.mod` that declares 1.26.6.

### First attempt failed, and this is what it was

`pnpm -r run build` before installing failed with `sh: tsdown: command not found` for
`kaos`, `kosong`, `minidb`, `oauth`. Those workspaces call `tsdown` in their build script,
but declare it only at the root, and their per-package dependencies had never been
installed in this copy — so the tool was not resolvable. The documented install step fixed
it; no config was changed. After install the same build exits 0.

### Corrections to checks made earlier in this session

- **A2 is closed**: `floyd-code/packages/klient/Dockerfile` IS tracked and is not ignored.
  The earlier check used the wrong root path.
- **A3 is closed**: `apps/floyd-code/dist-web` is present with 525 tracked entries. The gate
  script lives at `floyd-code/apps/floyd-code/scripts/check-web-assets.mjs`, not `scripts/`.

### Added

- `FLOYD.md` at the project root: identity, operating rules, and the order of work, so a
  session opening in this folder does not need telling what the project is. This is also the
  file phase B3 will read.

- 2026-10-19 ~01:20: checkpoint written (process fix). Wave 3 still running. Web/backend correction recorded: exists, ships as-is, nothing rebuilt.
- 2026-10-19 ~02:30: OWNER-CONFIRMED GAP. Intelligence layer content was never ported: donor-only templates (floyd_protocol, floyd-general, protocol_kernel, superfloyd-coder, mcp_tools_reference, deterministic/) absent; 7 same-name templates differ; prompt assembly (prompt.go vs prompts.go) unreconciled; chassis runs stock Crush coder.md.tpl; prompt-pack is inert. Fix work dispatched (work/intelligence-prompt). Verification must be behavioral: assembled-prompt diff vs donor, not file presence.
- 2026-10-19 ~02:30: INTELLIGENCE LAYER PORTED AND MERGED (work/intelligence-prompt, c8550b5).
  Templates byte-identical to donor (8/8 shasum MATCH): protocol_kernel, floyd_protocol,
  floyd-general, superfloyd-coder, initialize, summary, title, agentic_fetch + deterministic/.
  Kernel injected as its own system message after prefix, before role. Dynamic context
  (cwd/git/date/.supercache) now built via BuildCacheablePrompts (was dead code) and injected
  uncached. PromptDat extended with AvailMCPXML/AvailPluginXML/PluginInstrXML (empty until
  chassis registry exposes XML helpers; runtime MCP instructions already injected separately).
  Gates: build 0; agent suite -race green; render + selection + kernel tests PASS.
  Binary rebuilt: dist/darwin-arm64/megafloyd, version c3d2f6f, smoke OK.
  UNVERIFIED: live provider session (no credentials exercised), real API-call kernel position.

## 2026-10-19 — B6 cloud verification (fix/cloud)

### Devcontainer version stamp

`.devcontainer/post-create.sh` now computes the version with
`git describe --tags --always --dirty` (the same command as
`scripts/build-megafloyd.sh`), stamps `internal/version.Commit` as well as
`Version`, and passes `-buildvcs=false`. It exits non-zero if the built binary
prints `devel` or a version other than the computed one.

Real run with an isolated `HOME` and the shared Go module cache:

```text
$ GOMODCACHE=/Users/floyd/go/pkg/mod GOCACHE=/Users/floyd/Library/Caches/go-build \
    HOME=/tmp/mfl-pc2-home.1ABa6Y bash .devcontainer/post-create.sh
megafloyd: building version b64a4fb-dirty (commit b64a4fb)
megafloyd: built /tmp/mfl-pc2-home.1ABa6Y/.megafloyd/bin/megafloyd
megafloyd version b64a4fb-dirty
POST_CREATE_EXIT=0
```

Post-commit rerun on the clean tree: exit 0, and the version line was
`megafloyd version b87b129` (the then-current short commit).

Negative test: the same script in a fake checkout with no `.git` and a stub
`go` that emits a binary printing `devel` exited 1 with
`megafloyd: ERROR: binary reports 'devel'; version and commit ldflags were not applied`.

### VM installer, executed for real

Docker: the CLI is installed, but `docker info` failed because no daemon was
reachable at `~/.orbstack/run/docker.sock`, and no OrbStack app was present
in `/Applications`. Lima is installed but had no VM image. Per the lane rule,
the installer was run with a temporary prefix instead. Nothing under
`/usr/local` was touched.

```text
$ bash scripts/install-megafloyd.sh --unsupported-os --no-service-user \
    --prefix /tmp/mfl-install-prefix.2sgXoO --source-dir "$PWD"
[megafloyd-install] platform: Darwin/arm64
[megafloyd-install] required Go version: 1.26.6
[megafloyd-install] WARNING: existing Go 1.25.7 is older than 1.26.6
[megafloyd-install] installing Go 1.26.6 (darwin-arm64) into .../libexec/megafloyd/toolchain
[megafloyd-install] building megafloyd b64a4fb-dirty for Darwin/arm64
[megafloyd-install] installed binary: .../libexec/megafloyd/megafloyd
[megafloyd-install] installed launcher: .../bin/megafloyd
[megafloyd-install] service user creation skipped (--no-service-user)
[megafloyd-install] verified: megafloyd version b64a4fb-dirty
INSTALL_EXIT=0
```

Launcher checks, all exit 0:

```text
$ .../bin/megafloyd --version
megafloyd version b64a4fb-dirty
$ .../bin/megafloyd --help          # prints USAGE, EXAMPLES, command list
$ .../bin/megafloyd dirs
/tmp/mfl-install-home.msxq57/.megafloyd/config
/tmp/mfl-install-home.msxq57/.megafloyd/data
/etc/floyd
```

A second run into the same prefix exited 0 and exercised the upgrade path:
it moved the old files to
`libexec/megafloyd/backup/binary.20261019032314` and
`libexec/megafloyd/backup/launcher.20261019032314`, then reinstalled and
re-verified.

Steps not executed on this host, because the run was not on Linux and not in
a container: `apt-get` prerequisite installation, `groupadd`/`useradd`
service-user creation (disabled by `--no-service-user`), the Linux Go tarball
download (the Darwin tarball was used), and installation of a Linux binary
into a Linux prefix. The build, install, backup, launcher, and verification
logic all ran for real.

The installer's final message points at `docs/cloud-vm.md`. That file is not
on `fix/cloud`; it exists on the `work/cloud-vm` worktree (branch
`work/cloud-vm`, commit `d8d602c`) and was read there for context.

### Linux binaries

`bash scripts/build-megafloyd.sh -t linux/amd64 -t linux/arm64` exited 0:
`dist/linux-amd64/megafloyd` (95,735,970 bytes) and
`dist/linux-arm64/megafloyd` (91,357,346 bytes), both statically linked ELF.
They were not executed: Homebrew has qemu 11.1.1, but the user-mode binaries
`qemu-x86_64` and `qemu-aarch64` are not present, and running them directly
on macOS returns exit 126 (`cannot execute binary file`).

### Gates

`bash -n .devcontainer/post-create.sh` 0; `shellcheck` 0;
`python3 -m json.tool` and `jq empty` on `devcontainer.json` 0;
`go build ./...` 0 (no Go changed).
- 2026-10-19 ~03:45: AUDIT REMEDIATION WAVES 3-4 MERGED. Jobs system (7 tools live),
  plan mode (tools, guard, /plan, reminders), /web + /remote-control, admin client
  (routes verified from server /platform.js; auth model open), cloud devcontainer
  stamped + installer executed in temp prefix, add-dir + theme + experiments shipped.
  Full race suite green; binary rebuilt (a051e7b); smoke: 46 commands installed,
  doctor/resilience/scoreboard/auth/admin helps exit 0.
  OPEN: admin auth key verification (owner key needed); /desktop target decision;
  Codespaces blocked on owner-authorized push; Linux installer paths need Docker/VM.

## 2026-10-19 — house lane checkpoint (fix/house)

Base `1d15945`. Closed the house lane: seven job command files, SSOT wave
status, this checkpoint, and review notes.

### Rewritten command files

`floyd-go/.floyd/commands/`: `contract.md`, `prove.md`, `drift.md`,
`why.md`, `recover.md`, `handoff.md`, `replay.md`.

They now describe the shipped tools instead of pre-tool templates:
`contract` (create/list/show/complete), `prove` (record/show), `drift`,
`why`, `recover`, `handoff`, and read-only `replay`. Removed the
`.harness/contracts/` JSON template, `transcript_export`, session-fork, and
PROVEN/REFUTED prose. Storage is `<data>/jobs/<job_id>/`; verdicts are
`passed`/`failed`/`unchecked`.

Tool names verified:

```text
$ grep -n 'ToolName = ' internal/agent/tools/jobs_*.go
jobs_contract.go:15:const ContractToolName = "contract"
jobs_drift.go:15:const DriftToolName = "drift"
jobs_handoff.go:16:const HandoffToolName = "handoff"
jobs_prove.go:15:const ProveToolName = "prove"
jobs_recover.go:13:const RecoverToolName = "recover"
jobs_replay.go:13:const ReplayToolName = "replay"
jobs_why.go:14:const WhyToolName = "why"
```

Registration: `internal/agent/coordinator.go:818`; allowlist:
`internal/config/config.go:956-962`.

### Gates

| Gate | Command | Result |
|---|---|---|
| Build | `GOCACHE=/tmp/mfl-house-gocache go build ./...` | exit 0 |
| Targeted race | `GOCACHE=/tmp/mfl-house-gocache go test -race ./internal/jobs/... ./internal/agent/tools/... ./internal/commands/... ./internal/megafloyd/skills/...` | exit 0; 5 packages ok |
| Full race | `GOCACHE=/tmp/mfl-house-gocache go test -race ./...` | exit 0; 67 ok, 36 no test files, 0 FAIL |
| Format | `/Users/floyd/go/bin/gofumpt -l .` | exit 0; no files listed |

The default shared build cache was missing entries
(`/Users/floyd/Library/Caches/go-build/...: no such file or directory`), so
the gates ran with an isolated `GOCACHE`. No Go files changed.

### Known follow-up

`internal/megafloyd/skills/data/*.md` is the embedded copy installed at
startup and still holds the old seven files; it is outside this lane.
Starting the binary in this repo rewrites `.floyd/commands` from that embed
(`internal/app/app.go:175`, `internal/megafloyd/skills/install.go:148`).
The seven `skills.json` registry entries are stale too. See
`docs/review-notes.md`.

## 2026-10-22 — Super Floyd Source, tear-down, identity wiring (BUILD)

- Created `CaptainPhantasy/superfloyd-source` (private). Snapshot push `713c15a`
  (6,095 files; `.github/workflows/cloud-ci.yml` omitted — token lacks `workflow` scope).
  Clone-back tree hash `03ff56a4…` matches the pushed tree exactly.
- Redacted the PostHog key in `docs/MERGE-STATUS-20261016.md` before pushing (`d5da931`).
- Found the gap: `megafloydweb` never imported `internal/agent`; the Node agent rendered an
  82-line `system.md` with `role_additional` hardcoded to `''` (`profile-shared.ts:147`).
- Wired it (`1beed90`, branch `identity-wired` in the new repo): `WebRolePrompt()` composes
  kernel + protocol + role; `BuildChildEnv` passes it as `FLOYD_ROLE_ADDITIONAL=gz1:…`;
  `agent-core-v2` decodes it into `role_additional`.
- Obfuscated the layers in the artifact: templates are gzip+base64 blobs
  (`templates_blob.go`, generator `scripts/obfuscate-prompts.mjs`); binary `strings` shows
  0 hits for `Protocol Kernel` and `SOTA Full-Stack Architect`.
- Proof: Go tests pass; tsc 0 errors; vitest 4/4; rebuilt `dist/main.mjs` contains the decode;
  `make build` + `verify` 14/14 + `smoke` 3/3; engine `--web` served HTTP 200.
- Report: `docs/INSIDE-THE-SOURCE.md` (also on `docs-inside` in the new repo).
- Not wired yet: Go tools/skills vs TS tools/skills, hooks (TS only), web UI source
  (`code-app` repo absent), CI workflow.
