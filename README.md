# MEGAFLOYD

Work-in-progress merger of the two Floyd lines into one build. **Nothing here is wired to the
existing installs.** This is a scratch project: break it freely.

## Layout

| Path | What it is | Origin |
|---|---|---|
| `floyd-code/` | Floyd Code v2.0.2 — TypeScript/pnpm monorepo (CLI, `vis` web UI, `kap-server`, `acp-server`, oauth, vscode app) | copy of `/Users/floyd/floyd-v6/KimiSource/kimi-code` (its `.git`, `node_modules`, native/web dists excluded) |
| `floyd-go/` | Go agent — prompt/context assembly, skills, hooks, shell layer | copy of `/Users/floyd/floyd-v6/src` (`.git` excluded) |
| `docs/prompt-pack/` | The Go prompt templates + assembly reference, extracted as plain data | from `floyd-go/internal/agent/{templates,prompt}` |
| `PLAN.md` | Phases, plug points, gates, risks | written before this copy |

## Isolation rules

- Never point this project at `~/.floyd7`, `~/.floyd-ff`, `~/.floyd-code` or `~/.floyd-10x`.
  Use its own home dir (proposed: `~/.megafloyd`) when a merged build is ready to run.
- Do not copy `credentials` files from any existing install into this repo.
- Do not push secrets. `pnpm-lock.yaml` is fine; `.env*`, `credentials`, `*.db*` are ignored.

## Prerequisites (measured 2026-10-16)

| Tool | Required | This machine |
|---|---|---|
| node | `>=24.15.0` | v26.9.0 ✓ |
| pnpm | `10.33.0` (packageManager) | 10.25.0 — run `corepack enable` to fetch the pinned version |
| go | `1.26.6` (go.mod) | go1.25.7 — first `go build` downloads the toolchain via `GOTOOLCHAIN` |

`GOTOOLCHAIN` note: `floyd-go/go.mod` declares `go 1.26.6`, so a local go1.25.x triggers a
one-time toolchain download on first build (needs network). Set `GOTOOLCHAIN=local` to fail
fast instead of downloading.

Prompt-pack drift guard (run from repo root):

```sh
node scripts/verify-prompt-pack.mjs          # verify every copy matches the canonical pack
node scripts/verify-prompt-pack.mjs --sync   # push canonical -> Go tree after a template edit
```

## Build

Floyd Code half:

```sh
cd floyd-code
pnpm install          # re-creates node_modules (excluded from git)
pnpm -r run build
node apps/floyd-code/dist/main.mjs --version
```

Go half:

```sh
cd floyd-go
go build ./...        # toolchain go1.26.x is fetched automatically
go test ./internal/shell/...
```

## Status

Copied, not yet merged. P0 done, P1 (portable prompt pack) done — canonical pack +
drift guard + manifest. P2–P5 not started. See `PLAN.md` for the phase list and gates.

| Doc | What |
|---|---|
| `PLAN.md` | phases, corrected plug points, gates, risks |
| `docs/MERGE-STATUS-20261016.md` | point-in-time audit of P0/P1 |
| `docs/prompt-pack/MANIFEST.md` | canonical pack interface + checksums |
| `task_plan.md` / `findings.md` / `progress.md` | live working memory for the merge |
