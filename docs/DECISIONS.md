# DECISIONS.md — MEGAFLOYD

Open and closed decisions for the merge. One entry per decision; each names the gate it
unblocks. Nothing here changes code by itself.

---

## D-1 — `floyd-code/services/company-platform/` : IN or OUT of the merged build?

**Status:** IN THE TREE, OUT OF THE SHIPPED PACKAGE (corrected 2026-10-28). Was recorded as
"IN — SHIPPING (2026-10-16). Was OUT; owner directive reversed it, the egress review passed, and
the service is now named in `ship-manifest.json`." The directive to build the platform out still
stands; the manifest half of that sentence no longer does — see **Drift** below.

**Reversal (2026-10-16, owner verbatim):** "i have enjoyed the company platform… enough that I
recommended it to be included… i can always add things later. i want you to build out the
platform. it was missing your core ethics and ability to hold to a task and get it done."
The directory moves from provenance-only to a shipped surface. The egress review below is now a
**required step of build-out**, not an option.

**What it is:** a copied-in Python + JS + MJS service (`app.py`, `company_platform.py`,
`company-tools.mjs`, `gateway_bridge.mjs`, `media_tools.py`, `model_access.py`,
`feedback_store.py`, `platform.{html,css,js}`) — its own HTTP surface and model-access path.

**Original decision (superseded):** keep the directory in the tree (provenance), but **exclude
it from the P5 shipping build** and wire it to nothing; it stayed out pending its own egress
review.

**Rationale for reversal:**
- Owner-accepted surface; the console web UI is already the front door the owner uses.
- Its hardcoded internal defaults are removed (`be0e4f0`), so the egress objection is narrowed
  from "unknown endpoints baked in" to "enumerate the runtime network surface" — the build-out
  task below.

**Closed (build-out gate):** the egress surface is enumerated and enforced by the `EgressReview`
test (`test_platform.py`): only loopback and the overridable `models.dev` catalog default may
appear as literals, the outbound call-site count is fixed, and each sink must target configuration.
Remaining to fully ship: P5 packaging itself (launcher) — that is Track B/B4, not this decision.

**Drift (VERIFIED 2026-10-28).** The claim that the service "is listed in `ship-manifest.json`" is
no longer true. `ship-manifest.json` is now **manifest version 2** with `updated: 2026-10-25` and
`release_id: 2026.10.25-megafloyd-20261025T054728Z`; its `ship_set` holds five entries —
`bin/megafloyd`, the `MEGAFLOYD.app` bundle, the release `manifest.json`, the
`megafloyd-darwin-arm64.tar.gz` package and `install.sh` — and **no `company-platform` path**. Its
`note` states the position explicitly: "Complete canonical MEGAFLOYD package: the installed
launcher, bundled Go engine, web assets, and desktop shell. Existing company services remain
external shared services."

Read plainly: the 2026-10-16 owner directive still governs the source tree — the service stays at
`floyd-code/services/company-platform/` and its egress gate above still holds — but the shipped
package no longer contains it, and company services are declared external. Re-including it is a
build-scope decision, not a documentation fix; it needs a new owner-approved entry here and a
matching `ship-manifest.json` change (owned elsewhere). `scripts/verify-ship-manifest.mjs` was not
re-run for this record.

---


## D-2 — Source-repo secret: PostHog key still hardcoded upstream

**Status:** RECORDED — action owner-owned (outside this repo).

**Fact:** the merged `floyd-go/internal/event/event.go` is remediated (reads
`MEGAFLOYD_POSTHOG_KEY`, no key baked in). But the *source* repo still has it:
`floyd-v6/src/internal/event/event.go:18` → `key = "phc_4zt4…I0V"`.

**Decision:** do **not** edit the source repo from this project (isolation rule). Record it and
surface it as the one outstanding secret-hygiene gap. The merge is clean; the source is not.

**To close:** rotate the PostHog key and remove the literal from `floyd-v6/src`, or accept it as
already-public (it is a client-side PostHog key) and note that explicitly.

---

## D-3 — Prompt pack: one source of truth

**Status:** DECIDED — done.

`docs/prompt-pack/templates/` is canonical. `floyd-go/internal/agent/templates/` is a derived,
`//go:embed`-required copy (never edited directly). `../floyd-merge/prompt-pack/` is advisory,
outside git. Guard: `scripts/verify-prompt-pack.mjs` (PASS 2026-10-16).

---

## D-4 — `.gitignore` rules must not silently hide tracked-worthy files

**Status:** DECIDED — policy + applied for the root file (`f8c4541`); the last open instance is
**closed** (VERIFIED 2026-10-28).

**Rule:** bare directory/file names in `.gitignore` (`build/`, `logs/`, `credentials`,
`.floyd/`, `Dockerfile`, …) are recursive and WILL match source paths at arbitrary depth.
Where such a name is required as source/fixture, add an explicit `!` negation **after** the
excluding pattern (last-match-wins). Verify with `git check-ignore -v <path>` returning nothing
(or only negated matches) before committing.

**Applied:** root `.gitignore` negation block for `build/`, `logs/`, `credentials`, `.floyd/`
under `floyd-code/`.

**Closed 2026-10-28 (was "still open"):** the record here previously said
"`floyd-code/.gitignore:26` bare `Dockerfile` hides `packages/klient/Dockerfile`. To close: add
`!packages/klient/Dockerfile` there." Both halves of that line were stale: the bare `Dockerfile`
pattern is now at **`floyd-code/.gitignore:33`**, the negation is present at
**`floyd-code/.gitignore:38`** (with a comment at :36–37 explaining why), and
`git ls-files --error-unmatch floyd-code/packages/klient/Dockerfile` confirms the file is tracked.
The rule above is therefore applied — no instance remains.

**Guard (to add, Track A):** a script that diffs a source tree's tracked files against this
tree and fails on any path present upstream but absent here — turn the manual sweep into a gate.

---

## D-5 — `apps/floyd-code/dist-web/` is a committed artifact, restored not rebuilt

**Status:** DECIDED — restore from the source tree.

**Fact:** the prebuilt web bundle is developed in the upstream `code-app` repo and shipped here
committed (`floyd-code/AGENTS.md`). The MEGAFLOYD copy dropped it at copy time and the root
`.gitignore` ignores `dist-web/`; `check-web-assets.mjs` then aborts packaging.

**Decision:** copy `dist-web/` from the source checkout and **`git add -f`** it (keep the ignore
rule; force-add, exactly as upstream documents). Do not reimplement the web build here — the
toolchain for it lives in `code-app`, which is not present on this machine.

**To reverse:** if `code-app` becomes available locally, sync via
`FLOYD_CODE_REPO=<this checkout> pnpm run sync:web` and re-commit the bundle.

---

## D-6 — Internal host / private-IP references in committed text

**Status:** REMEDIATED (2026-10-16, `be0e4f0`).

**Rule (from the git-steward role):** every commit is treated as a permanent public artifact. Do
not commit internal hostnames, private IPs, or logins. Keep exact values in the local, gitignored
`.floyd/.supercache` and use placeholders (`<console-host>`, `<inference-host>`, `<login-user>`)
in tracked docs.

**Applied:** `findings.md` §I.1/§J — the console host, private IP, and login I had added
(`e9a3309`) are replaced with placeholders; the real values live only in `.floyd/.supercache`.

**Closed (vendored copies):** `floyd-code/services/company-platform/` shipped internal endpoints
as defaults in `company-tools.mjs`, `company_platform.py`, and `platform.js`. Sanitized in
`be0e4f0`: `company-tools.mjs` now requires `FLOYD_CODE_BASE_URL` / `FLOYD_CODE_CDN_BASE`; the
provider seed reads `company_model_base` or `FLOYD_COMPANY_MODEL_BASE`; the console install link
is server-injected (`window.__FLOYD_CONFIG`, `install_url` from `FLOYD_INSTALL_URL`). Post-fix
grep for the hostname/IP across the directory is empty and gitleaks is clean.

**History note:** the replaced values remain in commit `e9a3309`. Not worth a history rewrite for
a tailnet hostname in the owner's own org repo unless this repo is public — confirm visibility.

**Visibility (VERIFIED 2026-10-16):** `gh repo view CaptainPhantasy/MEGAFLOYD` → `isPrivate:true`,
`visibility:PRIVATE`. The tailnet host / private IP were never public; sanitizing is hygiene, not
incident response. No history rewrite (disproportionate for a private repo).

**CI (VERIFIED 2026-10-16, CORRECTED 2026-10-28):** the original record read "there is **no root
`.github/workflows/`**, so GitHub Actions runs nothing here… A push therefore carries no
CI-failure risk." That is no longer true. Root CI exists: **`.github/workflows/cloud-ci.yml`**
(name `cloud-ci`, added in `e457910`, 2026-10-19), triggered on `push`, `pull_request` and
`workflow_dispatch`, with `permissions: contents: read` and one job —
`linux/amd64 build, CLI smoke, installer` on `ubuntu-latest`, `timeout-minutes: 30`. A push to
this repo therefore **does** start CI.

What still holds from the original note: `floyd-code/.github` and `floyd-go/.github` remain inert
on this repo, because Actions only reads the repo-root `.github/workflows`. And the upstream
`if: … == 'LegacyAI'` guard warning still applies to any publish job added later: guard it on
`github.repository_owner == 'CaptainPhantasy'` instead. No publish step exists in the current
`cloud-ci.yml`, which runs read-only; that file is maintained by a separate owner and was not
edited here.

---

## D-7 — GUI is three independent verticals, not one interface with breakpoints

**Status:** DECIDED (owner directive 2026-10-16) — implementation deferred to Track F.

**Decision:** the GUI ships as **three independent verticals** — Desktop (autosizing between
standard and widescreen), Tablet (standard = iPad), Mobile (standard = iPhone 16 Pro). A viewport
width selects a *vertical implementation*; it does not switch branches of one shared responsive
stylesheet.

**Rationale:** owner directive. Practically it buys per-form-factor layout control (each vertical
optimised, no cross-breakpoint compromise) at the cost of three implementations to maintain and
three to verify — accepted deliberately.

**Spec + open points:** `docs/README-SPEC.md` (sizes; which web GUI to confirm). Screenshots come
from the **TUI** and the web GUI (owner clarified 2026-10-16).

**Gate:** `README-SPEC.md` gate section — three verticals render independently at the three
standard sizes; README written in the Floyd Labs voice with real live screenshots.

---

## D-8 — Where git branch/status disclosure lives in Floyd Code (B1)

**Status:** DECIDED — option (a), 2026-10-16. Design in `docs/B1-CONTEXT-ASSEMBLY.md`.

**Context (VERIFIED):** Go bakes `GitStatus` into `PromptDat`; Floyd Code discloses the *date* via
`features/dateChange` reminders and does **not** disclose git branch/status at all, though
`app/git/gitService.ts` can produce it.

**Decision:** mirror the codebase's own convention — a **session-start reminder** carrying branch
and a short status, not a `${git_status}` prompt variable. Consistent with `dateChange`, and it can
refresh when the tree changes, which a static prompt var cannot.

**First unit landed:** `src/features/gitStatus/gitStatusDisclosure.ts` (pure formatter) +
`test/features/gitStatus/gitStatusDisclosure.test.ts` (4 tests). Package typecheck + that test pass.
**Remaining:** the reminder actor + feature registration (design doc step 2–3).

---

## D-9 — The approved `MegaFloyd.md` revision: the fingerprint is pinned here, not in the document

**Status:** APPROVED by the owner 2026-10-28. This entry records the approval; it does not change
the approved document. `MegaFloyd.md` was not edited to create it.

**Approved standard:** `MegaFloyd.md` at the project root, 464 lines, SHA-256
`5bf0bb4a37990bfd1c7f7cc22c73e1ec2bfbfddac0d95952bae49652aa40945b`.

**Prior edition:** 468 lines, SHA-256
`6fe55c50a688d61cca15fda186ff3e91286099ef6b213d910a5d152e55ffa657`, whose own header read
"Policy version: 1.4. Revised: 2026-10-25 UTC." / "Status: approved by the owner for use in the
MegaFloyd harness."

**Nature of the difference — header removal only.** `diff -u` of the prior edition against the
approved one shows exactly one hunk: four lines deleted at the top and nothing else.

```
@@ -1,9 +1,5 @@
 # MegaFloyd — Operating Rules
 
-Policy version: 1.4. Revised: 2026-10-25 UTC.
-Status: approved by the owner for use in the MegaFloyd harness.
-Canonical filename: MegaFloyd.md. Purpose: standard behavior for every MegaFloyd agent and project.
-
```

That is the three provenance lines **plus the blank separator line** — 4 lines, 468 → 464. The
remaining text is byte-identical: hashing the approved file from line 3 and the prior file from
line 7 gives the same digest from both,
`cb8780b652628d317c5243f9ca7a0f345508a989ac97470a86bf68f148794ec5`. No rule was added, removed or
reworded.

**Owner decision:** do **not** reinsert the removed version header into `MegaFloyd.md`. A version
line inside the document is not the record of approval; this entry is.

**Where each edition is preserved** (all four paths verified 2026-10-28):

| Edition | sha256 | Location |
|---|---|---|
| approved, 464 lines | `5bf0bb4a37990bfd1c7f7cc22c73e1ec2bfbfddac0d95952bae49652aa40945b` | `MegaFloyd.md` (project root) |
| approved, 464 lines | `5bf0bb4a…` (identical) | `floyd-go/internal/agent/prompt/megafloyd_standard.md` — the `//go:embed` copy compiled into the Go binary |
| prior 1.4, 468 lines | `6fe55c50a688d61cca15fda186ff3e91286099ef6b213d910a5d152e55ffa657` | `releases/2026.10.25-megafloyd-20261026T023648Z/MEGAFLOYD.app/Contents/Resources/megafloyd/MegaFloyd.md` (the installed release, which `current` points at) |
| prior 1.4, 468 lines | `6fe55c50…` (identical) | `/Users/floyd/.megafloyd-recovery/20261028T134625Z/files/floyd-go/internal/agent/prompt/megafloyd_standard.md` (pre-repair recovery copy, outside the repo) |

Both copies of the approved bytes are the same file content in two places, which the prompt-pack
guard enforces as an assembly pair (`node scripts/verify-prompt-pack.mjs` → PASS). Of the seventeen
directories under `releases/`, only `2026.10.25-megafloyd-20261026T023648Z` contains a bundled
`MegaFloyd.md` at all, and it holds the **prior** edition — so the approved revision is currently a
source-tree pin, not an installed or shipped one. No completion record yet covers a
policy-bearing install; see `docs/megafloyd-completion.md`.

**Version history lives here, in `docs/DECISIONS.md` — not in `MegaFloyd.md`.** The pin is the
SHA-256 above. To identify the policy a build actually carries, compare
`prompt.StandardPolicySHA256()` (`floyd-go/internal/agent/prompt/prompt.go:34`, which hashes the
bytes embedded in that binary) against this table, or hash the file directly. A release's copy is
identified the same way.

**Rule — bumping the pin needs an explicit owner-approved revision.** The pinned fingerprint may
change only through a revision the owner approves and records as a new entry here, stating what
changed and why. A rebuild, a re-package, a `--sync` of the derived copy, a release that embeds a
different file, or a documentation edit is **not** a revision and must be corrected to match the
pin instead. If a build, release or document disagrees with the fingerprint above, the pin wins and
the drift is a defect to report, not a new version.

**Constraints recorded with the approval:** `MegaFloyd.md`, `AGENTS.md` and `claude.md` are not
edited by this bookkeeping; the removed header stays removed; and no content was reordered or
"tidied" while recording the pin.
