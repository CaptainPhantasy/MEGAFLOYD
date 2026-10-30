# MegaFloyd solidification plan — audit of 2026-10-27

Scope: the whole platform at branch `codex/megafloyd-modern-hooks` (HEAD `0983a9f`, in parity with origin) plus the uncommitted policy-activation changeset (38 modified files, 6 untracked files).

Method: a 16-agent audit workflow (7 subsystem mappers, 8 adversarial verifiers, 1 completeness critic; 575 tool calls) plus direct re-verification of the material claims in this session. Every item below carries its evidence label:

- **[EXECUTED]** — the command ran in this session and the output is quoted or summarized here.
- **[CONFIRMED]** — an auditor claimed it and an independent adversarial verifier reproduced the evidence this session.
- **[REPORTED]** — auditor-claimed with file:line evidence, not independently re-verified; re-check before acting.
- **[UNKNOWN]** — not established this session.

Nothing in this plan deletes anything. Removal is quarantine-first; permanent deletion requires the owner's two confirmations (MegaFloyd.md §11).

---

## 0. How "done" is defined here

MegaFloyd.md §1 (lines 29–41) fixes the state ladder this plan uses. A part of the platform is only as done as its state:

| State | Required evidence |
|---|---|
| Requested | The owner's actual instruction |
| Planned | A proposed action and acceptance condition |
| Implemented | The identified source/artifact contains the change |
| Tested | A named check ran against an identified version, with its result |
| Installed | The intended account's installation contains that version |
| Running | The observed process is using that version |
| Verified end to end | The intended user flow succeeded through its real dependencies |

Each fix below has a **DoD (definition of completion)**: the exact runnable check that moves the item to *Tested*. Phase 3 defines the checks that move the policy itself to *Installed/Running/Verified end to end*. "Compiles" and "health ok" are adjacent-layer facts, not completion (MegaFloyd.md §18/D1).

The platform is **solidified** when all of these hold at once:

1. A fresh clone of the branch builds and passes every repo gate (§4, Phase 1).
2. `go test ./...` in `floyd-go/` is fully green; the TS comment-free-zone lint is green.
3. The policy standard is pinned by hash in the ship manifest, checked at startup, and the guards run in CI automatically.
4. The installed `current` release carries the policy component and its binary reports the pinned hash.
5. The repo's records (task_plan, progress, findings, DECISIONS) agree with observed reality.
6. Cleanup is quarantined with move records, not deleted.

---

## 1. How the platform is intended to operate

The contract each subsystem is held to (sources: root `AGENTS.md`, `MegaFloyd.md` v1.4, `task_plan.md`, `docs/DECISIONS.md`):

| Part | Intended operation |
|---|---|
| Policy standard | Repo-root `MegaFloyd.md` is the ONLY external behavioral policy. The Go binary embeds those exact bytes (`megafloyd_standard.md`), renders them into every coder/task/initialize prompt via `{{.StandardPolicy}}`, binds source↔bundle by content hash in the build/release manifest, and checks the loaded version at startup. Old rulebooks (FLOYD.md, AGENTS.md, CLAUDE.md) are readable as data, never injected as authority (MegaFloyd.md §1). |
| Prompt pack | `docs/prompt-pack/templates/` is canonical; `floyd-go/internal/agent/templates/` is a derived `//go:embed` copy; `scripts/verify-prompt-pack.mjs` proves they match and hash-binds the two assembly pairs (`MegaFloyd.md↔megafloyd_standard.md`, `prompt.go.reference↔prompt.go`). Edit canonical only; sync with `--sync`. |
| Hook lifecycle | Config-declared hooks run across 9 lifecycle events through `internal/hookruntime`; PreToolUse runs BEFORE tool effects (deny/halt/rewrite/pre-approve); §17 requires before-action boundaries for rules that can be checked in code — notably §11's two-confirmation gate for permanent deletion. |
| Config & isolation | Merged home `~/.megafloyd`; old harness homes (`~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code`, `~/.floyd-10x`) never touched; project `.megafloyd` data under the active identity; context paths filter to MegaFloyd.md-named files only; `InitializeAs` forced to `MegaFloyd.md`. |
| Skills pack | `internal/megafloyd/skills/data/*` + `skills.json` are `//go:embed`ed and installed rewrite-only-when-changed into `<DataDirectory>/commands` at every startup; registry and disk must agree; skill text defers policy to MegaFloyd.md. |
| TS half (`floyd-code/`) | Second engine + committed prebuilt web runtime (`dist-web` 567 files, `dist/main.mjs`, `dist/search-worker.mjs` — committed on purpose). `agent-core-v2` decodes the Go-shipped standard (`FLOYD_ROLE_ADDITIONAL`, gz1). Packages `agent-core-v2`, `kap-server`, `transcript` are comment-free lint zones. |
| Release pipeline | `desktop/desktop/scripts/build.mjs` bundles the app + policy and pins hashes into provenance/receipts; `scripts/megafloyd-release.py` byte-compares bundled vs source policy, verifies the binary's embedded hash, and writes `components.policy` into the release manifest; `ship-manifest.json` is the ship-of-record; the installer verifies before repointing `current`. |
| Web child session | `WebRolePrompt` = protocol kernel + standard, passed to the TS child via `FLOYD_ROLE_ADDITIONAL`; both halves must assemble the same policy. |

---

## 2. What is already solid (verified this session — do not redo)

- **[EXECUTED]** Policy source = bundle: `shasum -a 256 MegaFloyd.md floyd-go/internal/agent/prompt/megafloyd_standard.md` → both `6fe55c50a688d61cca15fda186ff3e91286099ef6b213d910a5d152e55ffa657` (468 lines / 88,972 bytes; diff empty).
- **[EXECUTED]** `node scripts/verify-prompt-pack.mjs` → PASS, 15-file canonical pack, both assembly pairs hash-bound. `go build ./...` (floyd-go) → exit 0.
- **[CONFIRMED]** Embed + injection live: `prompt.go:27–34` embeds the standard, `:352–367` injects once with content-dedup of project/global copies; all four policy-bearing templates render `{{.StandardPolicy}}`; every role alias is forced onto megafloyd-coder (`prompts.go:72–74`); web child receives kernel+standard via `megafloydweb/env.go:39–53`. `go run . prompt policy --sha256` from this tree prints the pinned hash.
- **[CONFIRMED]** Contradictory inherited behavioral text removed: the changeset deletes ~1,562 lines of old rulebook content (`<critical_rules>`, old workflow/communication blocks); grep across canonical templates + modified Go prompt files finds zero rulebook hits; `templates_blob.go` re-verified in sync with on-disk templates; config now filters ContextPaths to MegaFloyd.md-named files and forces `InitializeAs='MegaFloyd.md'` (`load.go:594–703`).
- **[CONFIRMED]** Release chain exists and proved itself on release `2026.10.25-megafloyd-20261026T023648Z`: its binary prints the pinned hash, its bundled `MegaFloyd.md` matches, its manifest has `components.policy`; `megafloyd-release.py` byte-compares bundle vs source, runs the binary hash check, and re-verifies after install; `build.mjs` pins the policy hash into provenance/build receipts.
- **[CONFIRMED]** Hook lifecycle wired across all 9 events: per-event runners cached by config SHA, PreToolUse dispatched before tool effects at `coordinator.go:992` with deny/halt/rewrite/pre-approve semantics, terminal events observed with deadlock fixes from `0983a9f`.
- **[CONFIRMED]** Isolation intact: `TestCanonicalMegafloydConfigIsolation` passes; old homes excluded; project `floyd.json`/`floydrc` not initialized.
- **[EXECUTED]** Skills registry matches disk: 43 registry entries ↔ 41 md files, zero missing/unregistered; policy-bearing skills rewritten in this changeset to defer to MegaFloyd.md.
- **[EXECUTED]** Web runtime intact: `check-web-assets.mjs` → "567 files verified", exit 0; exactly 2 tracked dist files + 567 tracked dist-web files; agent-core-v2 typecheck exit 0; gz1 decode tests 4/4.
- **[EXECUTED]** gitleaks over all 189 commits: no leaks. Diff + untracked files: no credentials.

---

## 3. The confirmed defects (fix these; details and DoD in §4)

| ID | Severity | Defect | Verified |
|---|---|---|---|
| D1 | high | Six untracked files make the changeset a commit-trap: `git commit -am` bricks the build, the guard, and CI; the reviewed standard exists nowhere but this machine | CONFIRMED (reframed: committed HEAD is fine; the trap is at commit time) |
| D2 | high | §11 permanent-deletion two-confirmation is not enforced by any before-action hook — only prompt text and a generic single approval | CONFIRMED |
| D3 | high | `go test ./...` red at committed HEAD: 14 failing tests in 7 packages (baseline below) | CONFIRMED + [EXECUTED] |
| D4 | high | TS comment-free-zone lint red at committed HEAD: 6 violations in `agent-core-v2`; CI lint job fails | CONFIRMED |
| D5 | high | `ship-manifest.json` still points at pre-policy release `…T054728Z`, and its verifier only checks path existence — it cannot detect this | CONFIRMED |
| D6 | high | Installed `current` regressed to pre-policy release `…T012108Z` (install-receipt: 2026-10-26T22:45 demoted the policy-bound `…T023648Z`); users are running a binary with no policy and no `prompt policy` subcommand | CONFIRMED + [EXECUTED] |
| D7 | medium | §1 "check the loaded version at startup" not implemented — `StandardPolicySHA256()` has no startup caller | CONFIRMED (twice, independently) |
| D8 | medium | No pinned hash of the owner-approved standard: a coordinated edit of source + bundle passes every current guard | CONFIRMED (as part of D5 verification) |
| D9 | medium | Guards wired into nothing: cloud-ci runs no tests, no verify scripts, no policy-hash check; D-6's "no root CI" record is stale (`.github/workflows/cloud-ci.yml` exists since `e457910`) | REPORTED + [EXECUTED] (file existence) |
| D10 | medium | Config surface contradictions: `initialize_as` advertised as user-configurable but silently forced; `ProjectNeedsInitialization` still honors old rulebooks and doesn't recognize MegaFloyd.md; dead `defaultInitializeAs="AGENTS.md"`; stale `global_context_paths` schema default | REPORTED |
| D11 | medium | Runtime remnants of retired rulebooks: project-init keys off CLAUDE.md/FLOYD.md/AGENTS.md; SuperFloyd consistency lock still enforces a FLOYD.md boot contract; dead template decodes (`floydProtocolTmpl`, `coderPromptTmpl`) and dead role templates still shipped; old `.harness/` convention in 4+ skill files; stale skills.json `load_paths`; `doctor.md` says `megafloyd doctor` "arrives with wave 3" (it exists) | REPORTED |
| D12 | medium | memory.Writer git pathspec hardcodes `.floyd` while the checkpoint lives in `.megafloyd` under the active identity — the harness pollutes the git summary it records | REPORTED |
| D13 | medium | No prune of installed skill commands: skills removed from data/ persist forever in `<DataDirectory>/commands` | REPORTED |
| D14 | medium | TS/Go policy divergence: `FLOYD_ROLE_ADDITIONAL` reaches only the default TS `agent` profile (coder/explore/plan/tower never see the standard); TS half still auto-loads the AGENTS.md family into system prompts and re-injects it mid-session, contra §1 | REPORTED |
| D15 | medium | `desktop/build.mjs` records a policy hash without verifying the platform binary embeds the same policy (fails late at release time instead of fast at build time) | REPORTED |
| D16 | medium | `docs/prompt-pack/MANIFEST.md` stale: wrong checksums/count, missing the new assembly pair, dead paths | REPORTED |
| D17 | medium | `megafloyd-release.py` policy check is conditional — pre-policy manifests verify cleanly and silently; `source_receipt` inputs omit repo-root `MegaFloyd.md` | REPORTED |
| D18 | medium | DECISIONS drift: D-1 cites ship-manifest listing company-platform (no longer true in manifest v2); D-6 says "no root CI" (false); D-4 still open (actually closed: negation `!packages/klient/Dockerfile` present, file tracked) | REPORTED + [EXECUTED] (D-4) |
| D19 | medium | `docs/megafloyd-completion.md` pinned to the older release; no completion doc exists for a policy-bearing install; PR/CI claims in it are now unverifiable | REPORTED |
| D20 | medium | Company-platform egress gate not re-run on this branch; its ship/external status contradicts manifest v2 | REPORTED |
| D21 | low | ~1009 `.bak`/`.bak-release` items (none tracked): editing-session debris (floyd-go 571, floyd-code 232, desktop 62, scripts 24, docs 16, root docs/planning baks) PLUS installer-owned release backups (~64 `current.*`, `MEGAFLOYD.app.*`, `install-receipt.json.*`, `ship-manifest.json.*`) PLUS 85 `.bak-*` inside the committed dist-web assets dir (packaging-manifest miscount risk) | [EXECUTED] (count + classification) |
| D22 | low | Stale planning records: task_plan Track A table says A2/A3 open (closed per progress); Track E/F framing predates the release; findings §H table same | REPORTED |
| D23 | low | Dead legacy hook paths (agent.go:829–848, :962–969, `hooked_tool.go`); dead config metadata | REPORTED |

### The authoritative red/green baseline [EXECUTED]

`go test ./...` in `floyd-go/`, working tree (2026-10-27): **15 failing tests across 8 packages; 69 packages ok.**
Same command against committed HEAD `0983a9f` in a throwaway worktree: **14 of the 15 already fail at HEAD.** Only `TestPreparePrompt_FiltersImageAttachments` is introduced by the uncommitted changeset.

| Package | Failing test | At HEAD? | Known cause |
|---|---|---|---|
| internal/config | TestConfig_setupAgentsWithDisabledTools, …EveryReadOnlyToolDisabled | yes | expected tool lists missing `goal`, `hook_explain`, `hook_trace`, `run_skill` (load_test.go:833, :859) |
| internal/megafloyd | TestResolveIgnoresInheritedChassisDirs, TestChassisEnvDirectories, TestApplyChassisEnvOverwritesInheritedDirectories, TestApplyChassisEnvIsIdempotent | yes | commit `8c4876a` changed chassis-env/dir semantics without reconciling tests + doc comments |
| internal/agent | TestCanonicalRuntimeRefresh, TestRunSubAgent, TestBuildToolsRegistersCompleteToolSet | yes | WorkspaceDataDir `.megafloyd`-vs-registry inconsistency; 51-tool expected list stale; subagent cost-update error path undiagnosed |
| internal/agent | TestPreparePrompt_FiltersImageAttachments | **no — working tree** | preparePrompt changed attachment shape; test not updated |
| internal/agent/tools | TestCanonicalScratchpadAndJobsIsolation | yes | same WorkspaceDataDir decision as above |
| internal/agent/tools/mcp | TestCreateTransport_HeadersResolution | yes | undiagnosed |
| internal/app | TestModernHooksAppPermissionsAndShutdown | yes | undiagnosed (modern-hooks lane) |
| internal/backend | TestModernHooksDirectShellUsesWorkspaceDispatcher | yes | undiagnosed (modern-hooks lane) |
| internal/cmd | TestBuildAuthRuntimeRequiresHost | yes | undiagnosed |

TS: `node scripts/check-no-comments.mjs` → exit 1, 6 violations (`gitStatusDisclosure.ts:3,19`; `profile-shared.ts:206–209`), all from committed commits `905c627`/`1beed90`. [CONFIRMED]

---

## 4. The ordered work plan

Work the phases in order. Within a phase, items are independent unless noted. Every phase ends with its gate green before the next begins. Commit messages stay honest; nothing is pushed without the owner's explicit instruction (working agreement, task_plan.md:157).

### Phase 0 — Freeze the baseline (15 min)

0.1 Record the audit receipt: append a dated entry to `progress.md` pointing at this plan and the red/green baseline of §3. Quarantine nothing yet.
0.2 Confirm the six untracked files are intact and unmodified: `shasum -a 256 MegaFloyd.md floyd-go/internal/agent/prompt/megafloyd_standard.md` → both `6fe55c50…`.
   **DoD:** `progress.md` carries the entry; hashes match the pin above. If the hash differs, STOP — the standard changed since review; that is an owner decision (§5-Q1).

### Phase 1 — Make the changeset committable and the tree green

1.1 **Commit the policy activation as one atomic commit — explicitly adding the untracked six (D1).**
Never use `git commit -am` or `git add -u` here; both would commit the 38 modified files while omitting the 6 untracked ones they reference (`//go:embed megafloyd_standard.md` at prompt.go:27 fails to compile without it; `policy_root_*.go` define `policyRoot`; the tests and the guard reference `MegaFloyd.md`).

```
git add MegaFloyd.md \
  floyd-go/internal/agent/prompt/megafloyd_standard.md \
  floyd-go/internal/agent/prompt/policy_root_unix.go \
  floyd-go/internal/agent/prompt/policy_root_windows.go \
  floyd-go/internal/agent/prompt/megafloyd_policy_test.go \
  floyd-go/internal/config/megafloyd_policy_test.go
git add -A
git status --porcelain          # confirm nothing untracked remains
node scripts/verify-prompt-pack.mjs && (cd floyd-go && go build ./...)
git commit -m "feat: bind MegaFloyd v1.4 standard into prompt assembly and release chain"
```

   **DoD:** `git ls-files --error-unmatch MegaFloyd.md floyd-go/internal/agent/prompt/megafloyd_standard.md floyd-go/internal/agent/prompt/policy_root_unix.go floyd-go/internal/agent/prompt/policy_root_windows.go` exits 0 for all; guard + build pass **after** the commit; commit contains all 44 files.

1.2 **Fix the committed-red Go tests (D3).** One deliberate direction per group, tests and code reconciled together:

- config (load_test.go:833, :859): add `goal`, `hook_explain`, `hook_trace`, `run_skill` to both expected literals at the registry positions (after `todos` → goal; after `query_floyd_archive` → hook_explain, hook_trace; after `workflow` → run_skill). Preferred non-staleable variant: compute expected via the production `resolveAllowedTools(allToolNames(), cfg.Options.DisabledTools)` (config.go:1089, :996) instead of literals.
- megafloyd (4 chassis tests + doc comments): the post-`8c4876a` contract is deliberate — MEGAFLOYD_* directory vars always win and identity is always mirrored. Fix `dirs.go` CacheDir comment (drop the FLOYD_CACHE_DIR fallback claim), `env.go` ChassisEnv/ApplyChassisEnv docs (env.go:20–23, :43–46), then rewrite the four tests to assert the new contract (`TestResolveIgnoresInheritedChassisDirs` asserts megafloyd defaults win; expect `FLOYD_IDENTITY=DisplayName` when unset; `TestApplyChassisEnvOverwritesInheritedDirectories` expects inherited FLOYD_* dir vars to be overwritten; expect idempotency for dir vars + identity, untouched profile/skills).
- agent/tools WorkspaceDataDir: decide the canonical location for per-project agent state (project `.megafloyd` vs account-private registry — §5-Q2), make `PersonalDataDir`/`WorkspaceDataDir`/`manage_scratchpad`/`readSupercache` agree, then fix `TestCanonicalScratchpadAndJobsIsolation` and `TestCanonicalRuntimeRefresh`.
- agent tool-list test: update `TestBuildToolsRegistersCompleteToolSet` to the current 51-tool set (or assert membership of the four new tools).
- agent attachment test: reconcile `preparePrompt` with `TestPreparePrompt_FiltersImageAttachments` — either restore the attachment reminder as its own history item or update the test with a comment naming the new canonical shape.
- Undiagnosed trio: root-cause `TestRunSubAgent` (why `sql: no rows` escapes the best-effort `updateParentSessionCost` at coordinator.go:1729–1733), `TestCreateTransport_HeadersResolution` (mcp transport wrapping order), `TestBuildAuthRuntimeRequiresHost`, and the two modern-hooks tests (`internal/app`, `internal/backend`). Fix code or tests per findings; if any is environmental, document the required environment next to the test.

   **DoD:** `cd floyd-go && go test ./... -count=1` exits 0 with zero `--- FAIL` lines; `go vet ./...` clean.

1.3 **Fix the TS lint (D4).** Delete the 4-line `//` block at `profile-shared.ts:206–209`; convert the two JSDoc blocks at `gitStatusDisclosure.ts:3,19` into package-AGENTS.md notes or self-evident naming. No behavior change.
   **DoD:** `cd floyd-code && node scripts/check-no-comments.mjs` → exit 0; `pnpm --filter @legacy-ai/agent-core-v2 run typecheck` → exit 0.

1.4 **Reconcile config truth (D10, D23-low).** Update the `initialize_as` jsonschema tag to state it is fixed to `MegaFloyd.md` on this distribution; fix the `global_context_paths` default tag to `<configDir>/MegaFloyd.md`; delete dead `defaultInitializeAs`; add `MegaFloyd.md` recognition to `ProjectNeedsInitialization` (case-insensitive exact-name rule, reusing `policyName` logic) with a regression test (TempDir containing only `MegaFloyd.md` → returns false); retire the FLOYD.md boot-contract branch from the SuperFloyd consistency lock. Regenerate: `go run main.go schema > schema.json` (or the repo's `task schema`).
   **DoD:** `go run main.go schema | diff - schema.json` → empty; `go test ./internal/config/ -run 'NeedsInit|ContextPaths|PolicyDefaults'` → exit 0; `grep -rn defaultInitializeAs floyd-go --include='*.go' | grep -v .bak` → no hits.

1.5 **Regenerate stale docs of record (D16).** Rewrite `docs/prompt-pack/MANIFEST.md` from live files: 15-template checksum table, both assembly pairs (include `6fe55c50…`), blob pipeline (`obfuscate-prompts.mjs` → `templates_blob.go`), release-time policy chain; drop dead `/Users/floyd/...` paths or mark historical.
   **DoD:** `for f in docs/prompt-pack/templates/*; do case "$f" in *.bak*) continue;; esac; grep -q "$(shasum -a 256 "$f" | awk '{print $1}')" docs/prompt-pack/MANIFEST.md || echo "STALE $f"; done` → no output.

**Phase gate:** fresh-clone simulation — `git worktree add /tmp/mf-fresh <branch-tip>`, then in the worktree: `go build ./...`, `go test ./... -count=1`, `node scripts/verify-prompt-pack.mjs`, `node scripts/check-no-comments.mjs` (from floyd-code), all exit 0. Remove the worktree after. (This is the same receipt pattern used to produce §3's HEAD baseline.)

### Phase 2 — Close the structural policy gaps

2.1 **Startup loaded-version check (D7, §1 requirement).** At agent/app init: `slog.Info("loaded MegaFloyd standard", "sha256", agentprompt.StandardPolicySHA256())`; when a bundle standard exists adjacent to the binary (`Contents/Resources/megafloyd/MegaFloyd.md`) or at the configured global policy path, hash it and warn naming both hashes on mismatch. Warn, don't hard-fail: §1 calls this a build operation, and project policies are explicitly not hash-gated (MegaFloyd.md:19,124).
   **DoD:** launching the binary emits a log line containing `6fe55c50…`; tamper-test: change one byte of a fixture bundle's MegaFloyd.md → startup warns with both hashes; restore → clean line; `go test ./internal/cmd/ -run StartupPolicy` (new test) passes.

2.2 **Pin the approved standard (D8, D5-part).** Add a `policy` component (`path: MegaFloyd.md`, `sha256: 6fe55c50…`) to `ship-manifest.json`; extend `scripts/verify-ship-manifest.mjs`: for kind `release-manifest`, parse the referenced release manifest and require `components.policy.sha256` == sha256 of the checked-out `MegaFloyd.md`; fail with a message naming both hashes otherwise. Record in `docs/DECISIONS.md` (new D-9): bumping the pin requires an explicit owner-approved policy revision.
   **DoD:** `node scripts/verify-ship-manifest.mjs` exits 0 today; one-byte tamper of `MegaFloyd.md` without touching the manifest → exit 1 naming the mismatch; restore → exit 0.

2.3 **Wire the guards into CI (D9).** In `.github/workflows/cloud-ci.yml` after checkout add: `node scripts/verify-prompt-pack.mjs`; `node scripts/verify-ship-manifest.mjs`; `go test` (at minimum `./internal/agent/prompt/... ./internal/config/... ./internal/agent/...`); after build, compare `megafloyd prompt policy --sha256` output to `shasum -a 256 MegaFloyd.md`. Update D-6 to describe cloud-ci as the only live workflow and keep the nested-workflow publish-guard advice.
   **DoD:** on a scratch PR that edits `docs/prompt-pack/templates/coder.md.tpl` without `--sync`, CI fails at the verify step; a clean PR passes and prints the policy hash equal to the checkout's. (If running scratch PRs is not authorized, the local equivalents of every added step green + the workflow YAML lint is the interim DoD, and the CI receipt is deferred until the branch's own CI run.)

2.4 **§11 deletion enforcement (D2).** Add a before-action check on the bash/shell dispatch boundary (PreToolUse path in `hookruntime`, alongside `mechanical.go`): classify destructive commands by actual effect (rm/del/rmdir/git clean/DB drops per §11's classify-by-effect rule), default to quarantine, and enforce the two-confirmation sequence from private session records before allowing the effect — keeping the existing generic permission gate intact. Focused tests per §17: no deletion tool body executes before both confirmations; partial approved work resumes without new questions; completed actions are not repeated; unapproved additions are excluded (disposable fixtures).
   **DoD:** `go test -race ./internal/agent/... -run 'DeletionConfirm'` passes; `grep -n confirm floyd-go/internal/agent/tools/bash.go` shows the enforcement gate; the fixture tests prove no deletion starts before both confirmations are recorded.

2.5 **TS/Go policy composition decision + implementation (D14).** Owner decision first (§5-Q3), then one of: (a) prepend the inherited standard to every TS profile's `role_additional` (`profile-shared.ts:200`); (b) embed the standard in the shared system.md template; (c) record an explicit out-of-scope exception in DECISIONS. Same decision covers retiring AGENTS.md-family auto-loading in the MegaFloyd product path vs recording a §1 exception.
   **DoD (for a/b):** `npx vitest run packages/agent-core-v2/test/app/agentProfileCatalog` passes with assertions that every registered profile's rendered prompt contains a stubbed standard sentinel; `node scripts/check-no-comments.mjs` still exit 0. **DoD (for c):** DECISIONS entry records the exception with its source.

2.6 **Runtime remnant retirement (D11, D23).** Remove dead decodes (`floydProtocolTmpl`, `coderPromptTmpl`) with their unreachable templates (regenerate `templates_blob.go` via `scripts/obfuscate-prompts.mjs`; `mustDecodeTemplate` panics if files are removed without removing vars — do both); finish the `.harness/` → SSOT/`.megafloyd` migration in the 4 skill files + 2 registry notes (match the learn.md/plan.md pattern); fix `doctor.md:18` to the affirmative; update `skills.json` `load_paths` or delete the field; delete the dead legacy hook runner fields/branches + `hooked_tool.go` once its tests are folded in.
   **DoD:** `grep -rn '\.harness' floyd-go/internal/megafloyd/skills/data/` → no hits; `grep -n 'arrives with wave 3' …/doctor.md` → no hits; `go build ./...` + `go test -race ./internal/agent/... ./internal/megafloyd/...` green; `node scripts/verify-prompt-pack.mjs` green after template retirement.

2.7 **Skill-command pruning (D13) and checkpoint pathspec (D12).** Implement the manifest-based prune in `skills.InstallCommands` (write `<dataDir>/commands/.megafloyd-skills-manifest`; on the next run remove only files listed in a previous manifest that are no longer current — never touch foreign/user files). In `memory/writer.go`, derive the git-exclusion pathspec from the checkpoint's actual directory name instead of the `.floyd` constant.
   **DoD:** `go test ./internal/megafloyd/skills/...` green including a shrink-manifest test asserting a stale entry's file is gone while foreign files survive; `go test ./internal/megafloyd/memory/...` green including the active-identity pathspec subtest.

2.8 **Release-gate hardening (D15, D17).** In `desktop/desktop/scripts/build.mjs`, after resolving `PLATFORM_BIN` run `execFileSync(PLATFORM_BIN, ['prompt','policy','--sha256'])` and throw unless it equals the computed policy hash (missing subcommand = hard error naming the binary as pre-policy; optional `MEGAFLOYD_ALLOW_POLICY_MISMATCH=1` escape hatch). In `megafloyd-release.py`, make the `components.policy`-absent case emit a loud warning (it currently verifies cleanly as legacy); add repo-root `MegaFloyd.md` to `source_receipt` inputs.
   **DoD:** pointing the build at the old `012108Z` binary exits non-zero before staging; pointing at a current binary exits 0; a synthetic legacy manifest triggers the warning line.

**Phase gate:** `go test ./... -count=1` green; TS lint + typecheck green; verify-prompt-pack + verify-ship-manifest green; the tamper-tests of 2.1/2.2 demonstrated once and recorded in `progress.md`.

### Phase 3 — Ship the policy to the user (Installed → Running → Verified end to end)

3.1 **Confirm the install demotion (D6, §5-Q4).** The receipt says `installed_at 20261026T224556335083Z` set `current → releases/2026.10.25-megafloyd-20261025T012108Z` (pre-policy), demoting the policy-bound `…T023648Z`. If not an intentional rollback, reactivate the policy-bound release with its own installer (never hand-edit the `current` symlink — the installer owns backups/rollback):

```
python3 releases/2026.10.25-megafloyd-20261026T023648Z/install.py install \
  --prefix /Volumes/SanDisk1Tb/MEGAFLOYD \
  --manifest dist/releases/2026.10.25-megafloyd-20261026T023648Z/manifest.json \
  --archive dist/releases/2026.10.25-megafloyd-20261026T023648Z/megafloyd-darwin-arm64.tar.gz
```

then stop the idle runtime process (activation is `new-processes-only`) and let the desktop relaunch pick it up. If Phase 1–2 changed code, prefer cutting a **fresh** release from the landed tree instead of reactivating `023648Z`, and install that.

   **DoD:** `readlink current` → the policy-bearing release; `current/MEGAFLOYD.app/Contents/Resources/megafloyd/bin/megafloyd prompt policy --sha256` → `6fe55c50…`; `shasum -a 256 current/MEGAFLOYD.app/Contents/Resources/megafloyd/MegaFloyd.md` → same; `python3 -c "import json; print(sorted(json.load(open('current/release.json'))['components']))"` includes `policy`.

3.2 **Repoint + extend the ship-of-record (D5).** Repoint `ship-manifest.json` to the policy-bearing release (existing `023648Z` or the fresh Phase-3 release) and re-run the extended verifier from 2.2.
   **DoD:** the pinned-hash assertion of 2.2 passes against the referenced release manifest; `node scripts/verify-ship-manifest.mjs` → "ship manifest OK", exit 0.

3.3 **Actual user-flow check (Verified end to end).** Launch the installed app; confirm the startup log carries the loaded-standard hash (from 2.1); run one ordinary session turn through the terminal interface and observe the policy is the assembled one (`/prompt` inspection or the CLI dump). Then write a dated completion doc (`docs/megafloyd-solidification-completion.md`) following the repo's evidence-table convention, and update `ship-manifest.json`'s release_id if a fresh release was cut.
   **DoD:** the completion doc exists with per-requirement evidence rows (state ladder §0), and every row's check ran against the installed release this session.

### Phase 4 — Hygiene and records that tell the truth

4.1 **`.bak` cleanup proposal (D21) — quarantine, not deletion.** Split the 1,009 items into (a) installer-owned release backups (~64: `current.*.bak-release`, `MEGAFLOYD.app.*`, `install-receipt.json.*`, `ship-manifest.json.*`) — these are the installer's rollback mechanism; propose **keeping** them; (b) editing-session debris (`*.bak-<tag>`, `*.bak-final-delivery`, etc. across floyd-go 571, floyd-code 232, desktop 62, scripts 24, docs 16, root) — propose quarantining into `.quarantine/` preserving relative paths with a move-record manifest; (c) the 85 `.bak-*` inside `floyd-code/apps/floyd-code/dist-web/assets/` — same quarantine, but note they are inside a manifest-counted directory (the 561/567-file checks skip them today; verify the manifest gate before and after the move). Permanent deletion of anything happens only after the owner's two §11 confirmations.
   **DoD (quarantine variant):** `find . -name '*.bak*' -not -path '*/node_modules/*' -not -path '*/.git/*' | wc -l` drops by the number of move records in `.quarantine/MANIFEST.md`; `check-web-assets.mjs` still exit 0 after the dist-web moves; every moved item has a restore route.

4.2 **docs/ organization (MegaFloyd.md §6).** Reconcile `docs/` into `docs/plans/ docs/decisions/ docs/evidence/ docs/handoffs/ docs/reference/` with move records and updated links; keep intentional root entry points. Create `docs/MEGAFLOYD_SSOT.md` (canonical name per §5/P5) as the project single-source-of-truth, seeding it from this plan's §1/§2 and the location map — the SSOT records facts, never a second behavioral policy.
   **DoD:** every moved doc has a move record and its checked consumers (grep for links in code/scripts/docs) updated; `ls docs/` shows only intentional root-level files; the SSOT exists with Identity/Outcome/Location/Evidence sections.

4.3 **Decision and plan records (D18, D22, D19).** Update `docs/DECISIONS.md`: D-4 → CLOSED (negation present, `packages/klient/Dockerfile` tracked — [EXECUTED] verified); D-6 → corrected (cloud-ci exists; weaker than local gates); D-1 → amended per the company-platform outcome (4.4). Mark `docs/megafloyd-completion.md`'s PR/CI claims as historical/unverifiable if gh stays unauthenticated. Update `task_plan.md`/`findings.md`: A2/A3 → closed; add this plan as the active phase pointer.
   **DoD:** `grep -n 'Still open' docs/DECISIONS.md | grep -i dockerfile` → no hits; task_plan Track A table matches reality; no doc asserts "no root CI".

4.4 **Company-platform (D20).** Re-run the egress gate on this branch: `cd floyd-code/services/company-platform && python3 test_platform.py` → exit 0; then amend D-1 to state the actual v2 manifest status (external shared service vs packaged) and fix the stale "listed in ship-manifest.json" citation.
   **DoD:** egress test exit 0 on this branch; D-1 text matches the manifest v2 reality.

**Phase gate:** DECISIONS/plan files consistent with observed state; quarantine manifest exists; SSOT exists.

---

## 5. Owner decisions required (blockers only for their dependent items)

- **Q1 — Standard pin.** `6fe55c50…` (v1.4, 2026-10-25) becomes the pinned approved hash (2.2). Any future edit to `MegaFloyd.md` will fail the pin by design until an owner-approved revision bumps it. Correct? *(Default if unanswered: pin v1.4.)*
- **Q2 — Per-project agent state.** Project `.megafloyd/` or account-private registry for scratchpad/jobs/checkpoints? This decides the WorkspaceDataDir test fixes (1.2). *(Default: follow the already-committed resolver behavior — whichever of the two the code currently does wins, tests reconciled to it.)*
- **Q3 — TS-half policy composition.** Standard to every TS profile (a/b), or TS subagents recorded out of scope (c)? Also: retire AGENTS.md auto-loading in the TS path, or record a §1 exception? (2.5) *(Default: none — this one should not be defaulted, because it changes what subagents see.)*
- **Q4 — Install demotion.** Was the 2026-10-26 22:45 install of the pre-policy `012108Z` intentional? (3.1) *(Default if "not intentional": reactivate policy-bearing release.)*
- **Q5 — .bak disposition.** Quarantine editing debris + keep installer backups (4.1)? *(Default: quarantine proposal prepared, executed only on approval; nothing deleted.)*
- **Q6 — Push/PR.** Branch is 4+ commits ahead with a large changeset; `gh` reports the stored token invalid, so PR #1/#3 state is [UNKNOWN]. Pushing and PR work happen only on the owner's explicit instruction, after the Phase-1 gate.

## 6. Top-level completion matrix (fill as phases land)

| # | Item | Target state | DoD check | Status |
|---|---|---|---|---|
| 1 | Changeset committed atomically | Implemented + Tested | 1.1 DoD (fresh-clone build + guard) | ☐ |
| 2 | Go suite green | Tested | `go test ./... -count=1` exit 0 | ☐ |
| 3 | TS lint green | Tested | `check-no-comments.mjs` exit 0 | ☐ |
| 4 | Startup hash check | Implemented + Tested | 2.1 DoD incl. tamper test | ☐ |
| 5 | Hash pin + verifier | Implemented + Tested | 2.2 DoD incl. tamper test | ☐ |
| 6 | CI runs the guards | Implemented | 2.3 DoD | ☐ |
| 7 | §11 deletion enforcement | Implemented + Tested | 2.4 DoD fixture suite | ☐ |
| 8 | TS/Go composition rule | Implemented + Tested | 2.5 DoD per chosen option | ☐ |
| 9 | Policy-bearing release installed & running | Installed + Running | 3.1 DoD (readlink + binary hash + manifest component) | ☐ |
| 10 | Ship-of-record truthful | Implemented | 3.2 DoD | ☐ |
| 11 | Verified end to end + completion doc | Verified end to end | 3.3 DoD | ☐ |
| 12 | Quarantine + records truthful | Implemented | 4.1–4.4 DoDs | ☐ |

FINAL STATUS rule: any unchecked row with an actionable fix ⇒ INCOMPLETE (keep working); a row blocked only by an owner decision ⇒ BLOCKED on that item only, with the decision named.

## 7. Residual unknowns (recorded, not blocking)

- `policy_root_windows.go` behavioral parity: compiles/vets under GOOS=windows; no live Windows run [UNKNOWN].
- PR #1/#3 and remote CI state: `gh` token invalid (HTTP 401) this session [UNKNOWN].
- Root causes of `TestRunSubAgent`, `TestCreateTransport_HeadersResolution`, `TestBuildAuthRuntimeRequiresHost`, and the two modern-hooks failures: undiagnosed (Phase 1.2 final bullet).
- Non-audited breadth of `floyd-code` vendored apps (vscode 182, vis 125, floyd-inspect 75 tracked files): presumed upstream-vendored per the D-5 pattern [REPORTED].
- PostHog key in the out-of-repo source tree `floyd-v6/src` (D-2, owner-owned rotation): still open, outside this repo.

## 8. Session evidence index

Commands and outputs backing this plan (all run 2026-10-27): shasum pair + diff of `MegaFloyd.md`/`megafloyd_standard.md`; `node scripts/verify-prompt-pack.mjs` PASS; `go build ./...` exit 0; full `go test ./...` (15 FAIL / 69 ok) in the working tree and against HEAD `0983a9f` in worktree `/tmp/mf-head-check` (14 FAIL — removed after); `check-no-comments.mjs` 6 violations; `check-web-assets.mjs` 567 verified; `readlink current` + `install-receipt.json` (demotion receipt) + `dist/releases/` listing + `ship-manifest.json` release_id `…T054728Z`; bak inventory `find | wc -l` = 1009, none tracked; `gitleaks git` clean over 189 commits; `.github/workflows/cloud-ci.yml` exists (commit `e457910`). Subsystem detail and per-agent receipts: workflow run `wf_bd4b9b4b-c1e` journal (session transcript dir), summarized into §3/§4.
