<!-- owner-final-release-hold-20261024:start -->
**Release coordination — completed 2026-10-24 (UTC).**

The owner's earlier release hold was lifted after advance notice, the approved artwork checks and the explicit coordinated build instruction. Release `megafloyd-20261024T234343Z`, built from implementation commit `8c4876a3d166d9ac1ddd9952f27b2ab0bed3dfd3`, is installed, running and published. Actual icon restore, browser/desktop keyboard handoffs and the per-user startup job passed; the existing 23-message session, DeepSeek 384000/high, configuration and credential bytes were preserved.

The owner's requirement to give advance notice before a future final build remains in force. Historical worker notes below keep their original scope; they do not override this current release record. Final documentation and the ship-manifest pointer are recorded separately from the implementation, with no executable rebuild. [PR #1](https://github.com/CaptainPhantasy/MEGAFLOYD/pull/1) remains open and review follow-up is ongoing.
**Review follow-up:** The results below describe the installed `234343` release. PR #1 subsequently identified shared-runtime binding, subagent discovery, branch listing, batch-action, thinking-selection and goal-dispatch defects. Corrections are being verified in the review branch; an amended package and its installed acceptance remain pending. The earlier release receipts are retained with their original scope.

<!-- owner-final-release-hold-20261024:end -->

# task_plan.md — MEGAFLOYD: land the merge, then make it the tool

> Master plan. Supersedes the P0/P1-only framing. Kept in git so it travels with the project.

## Owner objective (verbatim, 2026-10-16)

- "complete MEGAFLOYD"
- "make the megafloyd package the ultimate tool for me but keep your core workethic in tact,
  its the best part of all of it and the part that was missing from the rest and what I came
  back for."
- standing: "ALSO FIND A WAY TO HOLD ON TO MEMORY. you keep losing it for some reason."
- standing: "vet everything before writing it as truth."

## Operating rules (the workethic — non-negotiable)

1. **Verify before asserting.** No claim of state without the command + its output.
2. **Evidence tags on every fact:** `VERIFIED` (ran it), `HYPOTHESIS` (reasoned, unrun),
   `LEAD` (heard/heuristically inferred — never promoted without a run).
3. **Memory persists in two places, both in git or the standard cache:**
   `task_plan.md` / `findings.md` / `progress.md` + `docs/DECISIONS.md` (tracked) and
   `.floyd/.supercache` (local cache). Checkpoint at every task boundary and mode switch.
4. **One hypothesis, one fix, observe.** On no change: invalidate, retract, do not loop.
5. **Gates are the only proof of "done".** A phase closes with a recorded gate result, never
   with prose.
6. **Never touch a source repo** (`floyd-v6/src`, `floyd-v6/KimiSource/kimi-code`). Read only.

## Where we are (2026-10-16, session 2)

- P0/P1 closed and committed at `4e85ae0`; build **relocated** to a fresh clone
  `/Volumes/SanDisk1Tb/MEGAFLOYD`.
- The fresh clone's `pnpm -r run build` failed **three times on the same defect class**:
  a required artifact exists in the working copy but is invisible to git, so it is absent
  from a clone. Root cause is a set of **over-broad, unanchored `.gitignore` rules** (plus one
  copy-time exclusion). Full inventory: `findings.md` §H.

| # | Artifact | Hidden by | State |
|---|---|---|---|
| 1 | `floyd-code/build/*.mjs` | root `build/` | **fixed** `9088cd4` |
| 2 | `apps/vis/web/src/components/logs/LogsTab.tsx` | root `logs/` | **fixed** `f8c4541` |
| 3 | `packages/agent-core-v2/src/human/credentials/*` | root `credentials` | **fixed** `f8c4541` |
| 4 | `migration-legacy/test/fixtures/**/.floyd/*` | root `.floyd/` | **fixed** `f8c4541` |
| 5 | `packages/klient/Dockerfile` | `floyd-code/.gitignore:26` bare `Dockerfile` | **open** |
| 6 | `apps/floyd-code/dist-web/**` (524 files) | copy exclusion + root `dist-web/` | **open** — blocks build gate |

Sweep proof (VERIFIED): `comm` of source-tree tracked files vs MEGAFLOYD tracked files under
`floyd-code/` = **525 paths = 524 × dist-web + 1 × klient/Dockerfile**. Nothing else is missing.

## Phases

### Track A — Reproducible tree (finish the P0 build proof)

| Phase | Task | Gate |
|---|---|---|
| A1 | Fix the ignore-rule defect class (instances 1–4) | `check-ignore` clean; files tracked | **done** `f8c4541` |
| A2 | Restore instance 5: force-add `packages/klient/Dockerfile` | file tracked; `git ls-files` shows it | open |
| A3 | Restore instance 6: copy `apps/floyd-code/dist-web/` from source, force-add | `check-web-assets.mjs` prints "Web assets OK" | open |
| A4 | Exhaustive "hidden artifact" sweep vs source tree | `comm` diff = ∅ beyond known exclusions | **done** (this plan) |
| A5 | Fresh-clone build proof: `pnpm install` → `pnpm -r run build` → `main.mjs --version` | exit 0 + version string | open |
| A6 | Record proof + update findings/progress/supercache; commit + push | gates table appended | open |

### Track B — The merge itself (P2–P5)

| Phase | Task | Gate |
|---|---|---|
| B1 (P2) | Context assembly in `agent-core-v2` — design + verified gap in `docs/B1-CONTEXT-ASSEMBLY.md`. Gap = **git branch/status disclosure** (date already via `dateChange` reminders). D-8 decided (reminder). Formatter unit landed; reminder actor pending | typecheck ✓ + vitest ✓ (unit); dumped prompt diffed against Go output (pending) |
| B2 (P3) | Port Go bash guards (no bare `<>`/`>|`/`&>|`, heredoc-in-substitution traps, read-before-write) to `externalHooks` rules | hook unit tests + denied-command demo |
| B3 (P4) | Identity + memory: `FLOYD.md` discovery + `.floyd/.supercache` read/injection + `FLOYD_*` markers | fresh-session smoke shows context loaded once, not per-turn |
| B4 (P5) | Package + ship: `floyd-one` launcher, own home dir, telemetry off | A/B session; no new egress beyond the inference host |

### Track C — Ecosystem surface (the "ultimate tool" reach)

Grounded in what already exists (from `floyd-code/AGENTS.md`): CLI/TUI, committed web UI
(`dist-web`), `vis` web GUI (sessions/replays), `floyd-inspect` (kap-server debug RPC),
vscode app, kap-server (REST+WS), remote-control tunnel, MCP gateway, slash commands.

| Phase | Task | Gate |
|---|---|---|
| C0 | **Discover** the actual plugin-dashboard / marketplace / MCP-gateway / slash-command code paths before designing (evidence, not assumption) | path inventory with `VERIFIED` tags |
| C1 | Wire the MCP gateway (MCP_SERVER_ROOM + secret-broker on inference) into the merged build | handshake succeeds; no secret in repo |
| C2 | Plugin dashboard + marketplace surface: catalog fetch + install path | install one plugin end-to-end in an isolated home |
| C3 | Slash commands + terminal + web GUI cohesion pass | one session exercised through all three surfaces |

Leads for C2 (UNVERIFIED, owner-supplied 2026-10-16): CDN "maybe" on Cloudflare; admin console
hosted by "indeeence". Do not treat as design inputs until confirmed by a run.

### Track D — Memory (answers the standing instruction)

| Phase | Task | Gate |
|---|---|---|
| D1 | Keep the tracked planning files + `.floyd/.supercache` checkpointed every session (already the practice) | each session ends with an updated checkpoint |
| D2 | Promote the "hold on to memory" wish into a product feature: `.supercache` auto-load + `FLOYD.md` (overlaps B3) | same gate as B3 |
| D3 | Decide where memory lives for the shipped tool (repo-local vs `~/.megafloyd`) | recorded in `docs/DECISIONS.md` |

### Track E — Company platform (IN build; owner directive 2026-10-16 — reverses D-1)

Service: `floyd-code/services/company-platform/` (Python stdlib console backend + member SPA).
Ship gate = **close D-1**: egress review, then add to the P5 build manifest.

| Phase | Task | Gate |
|---|---|---|
| E1 | First-run bootstrap so a clean checkout can start | clean-dir boot serves `/healthz` 200 **done** `491e1d7`; guarded `98db924` |
| E2 | De-hardcode internal endpoints + host-local paths (tool defaults, provider seed, install link, speech/ffmpeg/gateway/ComfyUI/llama-swap) | grep + gitleaks clean **done** `be0e4f0`, `86b7fb6` |
| E3 | Egress review: enumerate `gateway_bridge.mjs` / `model_access.py` relay / catalog fetch / feedback upload / media tools | **done** — enforced by `EgressReview` test `a3ae880` |
| E4 | Resume README gaps: portable-app command-window files; browser reply timing + reconnection checks | service-side done `cf976c6` (retry/backoff); command-window re-scoped to the app |
| E5 | Add service to the P5 build manifest; close D-1 | **done** — `ship-manifest.json` + verifier; D-1 = IN-shipping |

Tracks A/B/C/D below are unchanged and still pending except as noted (A2/A3/A5 landed in `62c0c0c`).

### Track F — README + GUI verticals (owner directive 2026-10-16; deliver at completion)

Spec: `docs/README-SPEC.md`; decision `docs/DECISIONS.md` D-7.

| Phase | Task | Gate |
|---|---|---|
| F1 | Confirm open points: which web GUI the verticals target; iPad standard size | recorded in the spec |
| F2 | Split that GUI into **three independent verticals** (Desktop autosize std↔widescreen; Tablet iPad; Mobile iPhone 16 Pro) — no shared-breakpoint interface | each vertical renders independently at its standard size |
| F3 | Capture real live screenshots: TUI + web GUI | images committed with alt text |
| F4 | Write `README.md` in the Floyd Labs brand voice | README present; spec gate met |

## Standing constraints

- Isolation: never point at `~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code`, `~/.floyd-10x`. Merged
  home dir = `~/.megafloyd`.
- Do not copy `credentials` from any install. Do not push secrets.
- `docs/prompt-pack/templates/` is the ONLY place templates are edited.
- Do not touch the source repos.

## Errors encountered (running log)

| Date | Error | Attempt | Resolution |
|---|---|---|---|
| 2026-10-16 | `Cannot find module .../floyd-code/build/raw-text-plugin.mjs` on clone | 1 | force-add helpers; root `build/` exception (`9088cd4`) |
| 2026-10-16 | `Could not resolve "../components/logs/LogsTab"` on clone | 1 | un-ignore four rule collisions (`f8c4541`) |
| 2026-10-16 | `check-web-assets.mjs`: missing committed `dist-web/` | — | open (A3) |

## Owner asks raised 2026-10-16, not yet scheduled (recorded here so they survive)

1. **Graphics card check — open.** The owner asked: is the speech model loaded, and on which
   card? He suspects a new router built two days ago may be erroring. Gate: a recorded answer
   with the command and its output in `progress.md`. Nothing in this project's files covered it.
2. **Audio edition standard for MEGAFLOYD reports.** Use the local speech service in the
   owner's own voice, not the paid one. Detailed, not padded, and under 10 minutes. Context for
   the length rule: an earlier run spent a month of paid audio credits on two reports, which is
   why the local service exists now.
3. **Method adopted (owner-supplied, now binding here).** The planning-files method: keep
   `task_plan.md` / `findings.md` / `progress.md` on disk, read them before deciding, log every
   error, never repeat a failed action, and treat a three-strike failure as a stop-and-report.
4. **Working agreement (owner-supplied, now binding here).** Before any push: scan for secrets
   and internal detail; keep commit messages honest; commit and push often.
5. **Delivery items already in Track F, restated by the owner:** README in the brand voice with
   real screenshots of the terminal interface and the web interface, and three independent
   layouts — desktop, iPad, iPhone 16 Pro — not one layout with breakpoints.
6. **Budget context.** This build will spend real money in cloud build spaces on the Flash model.
   Gates and evidence exist to keep that spend honest, not to slow it down.


<!-- owner-repair-plan-20261024:start -->
# MegaFloyd repair plan — 24 October 2026

The canonical project is /Volumes/SanDisk1Tb/MEGAFLOYD on the Mac mini.

Work down this numbered list. User clarifications supplement this plan. Keep the working Storage and SanDisk versions as the baseline, retain completed work, and fix demonstrated connection or ownership problems. Use old projects as references for feature behavior. Repurpose useful assets and retire old platforms when that saves time; legacy compatibility is not a requirement. Test only what directly establishes the canonical MegaFloyd result.

Intermediate builds are authorized. The owner has supplied the behavior examples and directed us to inspect the remaining dependencies ourselves. Continue source work and intermediate checks. The owner has approved the enlarged founder heads and authorized composer-head mouse following, focused verification, final packaging, updating the installed user copy, committing/pushing the completed changes, and a pull request with feedback follow-up. Give advance notice before the final build; no additional demo approval is required.

1. **Confirm the working baseline — recorded.**
   Canonical executable: /Volumes/SanDisk1Tb/MEGAFLOYD/bin/megafloyd.
   Canonical desktop: /Volumes/SanDisk1Tb/MEGAFLOYD/desktop/MEGAFLOYD.app.
   Alternate working desktop: /Volumes/Storage/MEGAFLOYD-DESKTOP/MEGAFLOYD.app.
   Existing desktop/UI changes are preserved. At the initial baseline, all three inspected Go executables reported “devel”, so version text alone could not distinguish their contents. The final release now has a version, unique BuildID, executable hashes and verified running paths, recorded in item 8.

2. **Finish MegaFloyd’s state separation — implemented; focused checks and live runtime identity passed.**
   Canonical home/configuration, workspace .megafloyd data, session database, cache, skills/roles, scratchpad, checkpoints, jobs and desktop lookup now resolve independently of inherited parent-harness paths. Existing donor histories and credentials have not been imported. Focused isolation checks passed.

3. **Finish SSH and inference gateway access — verified through the intermediate canonical agent.**
   SSH uses the normal Bash permission path. Canonical MCP configuration uses SSH stdio to inference. The rebuilt agent exposed eight gateway tools, obtained separate normal approvals, returned SSH hostname `inference`, and live-checked `environment-ecosystem`: reachable, 10 tools. Late discovery and MCP permission decoding defects were repaired and directly checked. See megafloyd-canonical-verification.json and megafloyd-mcp-onboarding.md.

4. **Finish the intelligence wiring — implemented with focused checks; live suggestion passed.**
   Role selection, fresh per-run context, tiered compaction, executable shell skills, persistent resilience and bounded AI suggestions have direct checks. The first live suggestion request failed on an expired Hyper token; native OAuth refresh succeeded, then the one bounded follow-up returned HTTP 200 in 3.395 seconds. At that earlier live check, the main model was opencode-go / beam-free and the suggestion model was hyper / deepseek-v4.1-flash. The owner subsequently selected opencode-go / deepseek-v4.1-flash and explicitly confirmed “Yes—keep DeepSeek”; the active workspace now retains its catalog maximum of 384,000 and high reasoning. See megafloyd-intelligence-evidence.md for direct evidence. The owner authorized the current Beam session configuration for completion work. The temporary saved test overrides have been removed: 131,072 maximum output tokens and medium reasoning are restored. Future tests bound their own duration without changing saved model settings.

5. **Finish terminal/web handoff and controls — implemented; focused and live runtime checks passed.**
   The actual terminal and browser use the same Go coordinator, workspace, database and session. A real web-submitted SSH turn completed and appeared in both interfaces. Presence counted two clients together, then one after browser closure. Browser completion and normal permission projection were corrected and directly checked. The owner instructed: “Don't record them as limits. Record the steps necessary to make them a possibility and do it.” Complete these steps within this item:

   5a. **Implemented; direct SQLite/file/HTTP checks passed.** Add Go-owned archive/unarchive, undo, fork/branch and per-turn file-history operations. Use existing session/message/history primitives; preserve earlier state before destructive changes. Wire the exact existing web contracts to those operations. Verified real state transitions, transcript recovery, and recorded file content. Undo matches the reference application: it rewinds conversation turns and leaves workspace files unchanged.

   5b. **Implemented; focused fixtures passed; live workspace identity passed.** Add workspace registration/selection, extra workspace roots and attachment upload/consumption. Store uploads under MegaFloyd's own workspace data and pass real files/content into the same Go agent. The direct fixtures cover path handling, isolation and attachment round trips; the live adapter preserved existing preferences and did not initialize historical workspaces while listing them.

   5c. **Implemented; direct control/queue/subagent/goal checks passed.** Expose and wire background task controls, queued prompts, subagent views/profiles, plans, swarm and goals through the existing Go execution paths. Persist their state within the canonical workspace. Verify creation, updates, cancellation and shared terminal/web observation without starting another agent engine.

   5d. **Implemented; direct checks passed.** Connect provider credentials/configuration, MCP configuration/reload, plugin management and admin controls. Web and native account access now use one MEGAFLOYD credential slot. MCP connections and events are isolated per workspace. Reuse existing validated Go config operations and pure management helpers. Apply plugin skills/prompts/MCP contributions to the Go coordinator and use the identified company assets deliberately. Verify persistence and actual runtime reload.

   **Model defaults corrected and verified.** A changed model now uses its own catalog maximum-output default and resets the previous model's provider options. Reasoning defaults to high when supported, otherwise the highest advertised effort; models with a simple reasoning switch default to on, and models without reasoning omit reasoning parameters. Explicit choices for the same model remain usable. Terminal reasoning off/on choices survive actual configuration reload, and the web status, session profile, snapshot and configuration consistently display an explicit off choice. The canonical Go reload/provider checks and focused web round-trip checks passed without generation. The current workspace preserves the owner-selected DeepSeek V4.1 Flash at 384,000/high, and the Small model remains 262,144/high. The older Beam 131,072/medium setting remains only in the global fallback configuration; it does not override the active workspace.

   5e. **Implemented; direct checks passed.** Implement all three visible approval policies, including Ask When Needed, in the Go permission path. Keep UI labels, actual policy and shared state consistent. Verify routine versus approval-requiring calls and normal grant/deny behavior.

   5f. **Go D and matching web bundle built; actual browser/terminal workflow passed.** Browser D2 performed SSH and gateway health through two ordinary UI approvals in 13.311 seconds and two assistant steps. The attached Go TUI displayed the same tool results and final response. The earlier browser attempt was canceled by the test guard at JSON-string MCP arguments after SSH succeeded; the adapter decoding was fixed and directly checked before D2. Cancellation also exposed a stale TUI approval dialog: the native terminal-notification fix now passes its focused regression and is included in the final build. The existing prebuilt UI and mascot were preserved. Final installed desktop and terminal verification is recorded in item 8.

   5g. **Direct launch and handoff commands — installed and verified.** Actual TUI/icon entry points and shared-session directions passed. The final browser and desktop keyboard `/tui` checks establish both correct terminal launch and cleared completion state on the combined release. Older menu/direction checks apply to unchanged modules; exact dated receipts are in the completion report.

   5h. **File Explorer locations — implemented and verified in the installed application.** Actual UI selection opened SanDisk, Storage and the mounted inference SMB share. The account-accessible protected Trash path returned200 without reading its item contents. The Explorer exposes volumes and direct paths subject to the account’s OS permissions; it does not depend on a donor project root. The earlier actual 224951 UI checks retain their timestamp; final 234343 read-only checks again passed SanDisk, Storage, the inference share and protected Trash access.

   5i. **Persistent macOS drive authorization — implemented and verified across installed relaunches.** Canonical app and native helpers use the same persistent signing certificate. The owner unlocked the macOS dialog and the actual canonical app was added to Full Disk Access using the supported system interface. Subsequent installed app filesystem checks opened external/network volumes and the protected Trash location without a new permission prompt. No privacy database was edited. The stable signature is carried into the final completion amendment.

   5j. **Canonical login startup and old Storage retirement — enabled and directly verified.** The per-account LaunchAgent executed under launchd with all six configured locations ready and the existing desktop process retained, without a duplicate. The old Storage executable login item is removed; stable volume UUIDs, saved SMB authorization and retry behavior are implemented. The Mac was not rebooted for this check. Physical old copies remain under the owner’s control.

   5k. **Independent account installation and state — implemented and directly checked.** Personal workspace data defaults to ~/.megafloyd/data/workspaces/<canonical-workspace-hash> with0700 directories; an explicit own-account mapping preserves the existing session store. Two account homes opening the same project do not share conversation data. Company model/API credentials and services may be shared intentionally. Focused checks cover home separation, symlink identity, explicit mappings and corrupted registry errors. The release excludes personal state and signing private material. Actual230715 also writes provider metadata to ~/.megafloyd/cache; old Floyd cache file metadata stayed unchanged.

   5l. **External dependency trace — inventory and fleet prompt delivered.** The owner is relocating old copies. Trace canonical installed/source dependencies by machine and absolute path, leaving those copies untouched. The report and TSV list 356 unique machine/path pairs; the evidence JSON separates configured service dependencies, personal state, system libraries, build inputs and optional gateway backends. All 91 package symlinks and 3,785 source symlinks stayed inside their respective canonical trees; 25 packaged native binaries declared bundled or system libraries. One real Mac build dependency was inherited Storage GOMODCACHE: canonical build/smoke scripts now use SanDisk's existing cache, while bare Go retains the user's global configuration. The configured inference model server genuinely loads nine libraries from /home/douglas/llama.cpp-src/build-b11177-full/bin; exact targets and 19 model files are recorded. Optional gateway backend transitive tracing is explicitly separate; the supplied fleet prompt assigns that work without inventing missing paths. See megafloyd-external-dependency-report.md and megafloyd-dependency-audit-fleet-prompt.md.

   5m. **Canonical MegaFloyd branding — installed and verified.** Terminal, desktop, browser, account portal and download entry points use MegaFloyd. The finished animated wordmark and founder-head revisions are included. All 561 authorized custom frontend assets match the checked source and installed package; earlier 560-file checks remain historical. Internal compatibility identifiers and owner conversation text are preserved as required.

   5n. **Founder-head sizing, gestures and pointer following — installed and verified.** All use sites double their baseline size. The composer-near head follows the pointer through eased directional sprite frames and gently settles on leave/blur. Other heads retain independent gestures and nearby expression coordination. Actual installed rendering, source direction checks and the lone-head idle-resume regression passed. The original sprite sheet and new animated wordmark are preserved.
6. **Run minimal intermediate checks — passed; receipts retained.**
   Build the changed canonical Go and web components into intermediate output paths.
   Verify canonical runtime identity, state location, gateway tool availability/execution, SSH, the bounded suggestion call, and terminal/web session continuity.
   Fix observed failures and repeat only the affected check.
   Use bounded test concurrency and observe open-file usage following the Mac incident.
   Record evidence separately from implementation claims.
   Eight reversible live control checks passed without a model call: plan editing, bounded swarm settings, per-session prompt/tools/MCP settings, manual shell history, fork/branch, archive/restore, undo, and idle queue/goal views. The three disposable sessions were removed. After the two authorized browser attempts, the original ten messages remain byte-equivalent by their content hash within the same 19-message session. Main/small model selections and provider configuration are unchanged. The browser submission's persisted default manual policy was removed through the native config API, restoring the original workspace config bytes. No further model calls are needed for final package verification.

7. **Incorporate the remaining inventory — complete for the supplied behavior brief and retained company integrations.**
   Verified path list: megafloyd-mac-verified-paths.txt.
   F7, Floyd7 and merge/export items explain intended feature behavior; they are not a literal import list.
   Several recent Floyd7 features already exist in canonical source, so copying those projects would duplicate work.
   Existing admin/CDN/MCP Room hosting is running. The CDN's current /code installer targets Floyd7 state. MEGAFLOYD's complete-package release/installer is now published under /megafloyd on the same hosting; the canonical installation and the installed updater's no-update check passed over HTTPS. The existing company marketplace was verified and its two official ZIP links repaired in place. See megafloyd-admin-cdn-inventory.md; existing hosting is reused without creating replacement services.
   Company account and marketplace URLs are now explicit keys in MEGAFLOYD's own config. Only those keys changed; model selections, provider credentials, and the native credential file were read back unchanged. Evaluate only relevant behavior gaps and explicitly retained company admin/CDN/gateway assets.
   One subsequent explicit SignIn on the installed release linked 18 assigned company models through the existing native account slot at https://inference.tail58d565.ts.net:8446/v1. This added only providers.managed:floyd-code with the company-auth placeholder; all earlier provider objects, selected models, credential bytes and 19 session messages remained unchanged. No model generation was performed during this link check.
   Feed any additional required changes back through the relevant numbered item without restarting this plan.

8. **Produce and verify the final build — complete for the canonical combined release.**
   Installed and running `megafloyd-20261024T234343Z` includes the finished 561-asset wordmark revision and corrected handoff completion server. Actual browser and desktop keyboard checks passed; all 23 messages, DeepSeek/high and protected config/credential bytes remain unchanged. Login helper activation and six-mount/no-duplicate execution passed. Package inventory/signature and release-feed checks are in the release report. Final checks used no model generation. See megafloyd-completion.md and megafloyd-installed-verification.json for timestamped results and retained historical failures.

9. **Commit, push and open the review — implementation pushed; PR open and CI passed.**
   Implementation commit `8c4876a3d166d9ac1ddd9952f27b2ab0bed3dfd3` is verified on remote branch `codex/megafloyd-canonical-completion`. [PR #1](https://github.com/CaptainPhantasy/MEGAFLOYD/pull/1) is open for review. The 561 custom frontend assets are preserved; personal state, credentials, backups and release packages are excluded. Final installation records and package metadata are recorded in this documentation commit, separately from the implementation and without rebuilding. Automated code/security reviews are running; follow and fix actionable findings. The PR has not been merged.

Committed summaries: [completion](docs/megafloyd-completion.md), [release verification](docs/megafloyd-release-verification.md), [startup and telemetry](docs/megafloyd-startup-telemetry-verification.md), [frontend preservation](docs/megafloyd-web-branding-preservation.md), and [MCP onboarding](docs/megafloyd-mcp-onboarding.md). Detailed receipt filenames mentioned above refer to retained local verification evidence; personal runtime JSON is not included in the repository.
<!-- owner-repair-plan-20261024:end -->
