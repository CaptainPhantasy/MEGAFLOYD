# DECISIONS.md — MEGAFLOYD

Open and closed decisions for the merge. One entry per decision; each names the gate it
unblocks. Nothing here changes code by itself.

---

## D-1 — `floyd-code/services/company-platform/` : IN or OUT of the merged build?

**Status:** DECIDED — **OUT** (recorded 2026-10-16; re-openable).

**What it is:** a copied-in Python + JS + MJS service (`app.py`, `company_platform.py`,
`company-tools.mjs`, `gateway_bridge.mjs`, `media_tools.py`, `model_access.py`,
`feedback_store.py`, `platform.{html,css,js}`) — its own HTTP surface and model-access path.

**Decision:** keep the directory in the tree (provenance), but **exclude it from the P5
shipping build** and wire it to nothing. It stays out until it gets its own egress review.

**Rationale:**
- `PLAN.md` P5 gate: "no new network egress beyond the existing inference host." This service
  adds `gateway_bridge.mjs` / `model_access.py` network surface that the P5 gate does not cover.
- The merged build's goal is the *Go agent behaviour layer* on the Floyd Code shell, not the
  company platform. Shipping it is scope, not completion.
- Reversible: it is one directory; keeping it untracked-by-build costs nothing.

**To reverse:** give it its own egress review, then add it to the P5 build manifest.

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

**Status:** PARTIALLY REMEDIATED — vendored copies left for owner decision.

**Rule (from the git-steward role):** every commit is treated as a permanent public artifact. Do
not commit internal hostnames, private IPs, or logins. Keep exact values in the local, gitignored
`.floyd/.supercache` and use placeholders (`<console-host>`, `<inference-host>`, `<login-user>`)
in tracked docs.

**Applied:** `findings.md` §I.1/§J — the console host, private IP, and login I had added
(`e9a3309`) are replaced with placeholders; the real values live only in `.floyd/.supercache`.

**Open (vendored, NOT introduced here):** `floyd-code/services/company-platform/`
(`company-tools.mjs:7`, `company_platform.py:93`) ships internal endpoints as defaults. The dir is
OUT of the ship build (D-1) but present in the tree. **To close:** either sanitize the defaults or
record them as accepted-and-internal; owner's call, since editing them diverges from upstream
provenance.

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
