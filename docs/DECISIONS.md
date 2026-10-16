# DECISIONS.md — MEGAFLOYD

Open and closed decisions for the merge. One entry per decision; each names the gate it
unblocks. Nothing here changes code by itself.

---

## D-1 — `floyd-code/services/company-platform/` : IN or OUT of the merged build?

**Status:** IN — SHIPPING (2026-10-16). Was OUT; owner directive reversed it, the egress review
passed, and the service is now named in `ship-manifest.json`.

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
The service is listed in `ship-manifest.json` (verified by `scripts/verify-ship-manifest.mjs`).
Remaining to fully ship: P5 packaging itself (launcher) — that is Track B/B4, not this decision.

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

**Status:** DECIDED — policy + applied for the root file (`f8c4541`).

**Rule:** bare directory/file names in `.gitignore` (`build/`, `logs/`, `credentials`,
`.floyd/`, `Dockerfile`, …) are recursive and WILL match source paths at arbitrary depth.
Where such a name is required as source/fixture, add an explicit `!` negation **after** the
excluding pattern (last-match-wins). Verify with `git check-ignore -v <path>` returning nothing
(or only negated matches) before committing.

**Applied:** root `.gitignore` negation block for `build/`, `logs/`, `credentials`, `.floyd/`
under `floyd-code/`. **Still open:** `floyd-code/.gitignore:26` bare `Dockerfile` hides
`packages/klient/Dockerfile`. **To close:** add `!packages/klient/Dockerfile` there.

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

**CI (VERIFIED 2026-10-16):** there is **no root `.github/workflows/`**, so GitHub Actions runs
nothing here — the copied `floyd-code/.github` and `floyd-go/.github` workflows are inert on this
repo (Actions only reads the repo-root `.github/workflows`). A push therefore carries no
CI-failure risk. If root CI is ever added, guard publish jobs on
`github.repository_owner == 'CaptainPhantasy'`; the upstream `if: … == 'LegacyAI'` guards would
otherwise skip every job.
