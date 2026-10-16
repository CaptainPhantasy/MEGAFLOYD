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
