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

Copied, not yet merged. See `PLAN.md` for the phase list and gates.
