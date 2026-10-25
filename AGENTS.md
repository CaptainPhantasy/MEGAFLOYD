# MegaFloyd — root agent guide

Two halves in one repo; there is no root manifest. Run TS commands in `floyd-code/` (pnpm) and Go commands in `floyd-go/` (go/task). Follow the nearest subdirectory `AGENTS.md` for half-specific rules (`floyd-code/AGENTS.md`, `floyd-go/AGENTS.md`, `floyd-go/internal/ui/AGENTS.md`).

## Isolation and secrets

- Never point this project at `~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code`, or `~/.floyd-10x`; merged home is `~/.megafloyd`.
- Do not copy credential files from any existing install. `.env*`, `credentials`, `*.db*` are gitignored.
- Do not push secrets or generated release/install products. The intentional
  committed web runtime includes `floyd-code/apps/floyd-code/dist-web` plus the
  existing `dist/main.mjs` and `dist/search-worker.mjs` in that same app. Keep
  these runtime files and their source changes together so a fresh clone can
  start the web interface.

## Prompt pack (one source of truth)

- Canonical: `docs/prompt-pack/templates/` (+ `docs/prompt-pack/prompt.go.reference`).
- Derived copy: `floyd-go/internal/agent/templates/` and `floyd-go/internal/agent/prompt/prompt.go` — never edit directly; regenerate with `node scripts/verify-prompt-pack.mjs --sync`.
- Verify with `node scripts/verify-prompt-pack.mjs` before claiming template changes.

## Web UI bundle

- Source is not in this repo; `floyd-code/apps/floyd-code/dist-web` is a committed prebuilt bundle.
- Sync from code-app with `FLOYD_CODE_REPO=<checkout> pnpm run sync:web` (run in `floyd-code/`), committing bundle changes together.
- Dev against it: `pnpm dev:server` here + `FLOYD_SERVER_URL` in code-app's `pnpm dev:web`.

## Build and verification

- Go: `scripts/build-megafloyd.sh` (CGO_ENABLED=0, GOEXPERIMENT=greenteagc) or `cd floyd-go && go build ./...`. First build may fetch Go 1.26 toolchain; set `GOTOOLCHAIN=local` to fail fast.
- TS: `cd floyd-code && pnpm install && pnpm -r run build`. Node `>=24.15.0`, pnpm `10.33.0` (engine-strict).
- Root checks: `node scripts/verify-prompt-pack.mjs`, `node scripts/verify-ship-manifest.mjs`.
- Generated code: `task sqlc`, `task schema`, `task swag`, `task hyper` (in `floyd-go/`); update goldens with `go test ./... -update`.
- TS comment-free zones (`packages/agent-core-v2`, `packages/kap-server`, `packages/transcript`) enforced by `pnpm lint`.

## Planning

- Master plan in `task_plan.md`; live memory in `progress.md`/`findings.md`; decisions in `docs/DECISIONS.md`. Work the next unfinished phase; do not invent work outside it.
