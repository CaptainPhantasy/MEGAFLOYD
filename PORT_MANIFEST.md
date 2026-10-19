# PORT_MANIFEST — megafloyd convergence

Owner objective: one new entity, launched as `megafloyd`, its own home dir, its own package.
Replaces nothing. Most modern chassis + every verified feature from all instances +
superfloyd v5.3 intelligence layer. Must install on local machines AND a cloud VM /
Codespace, reachable from a phone with zero home-network dependence.

## Canonical sources (read-only donors — never edited)

- Chassis (host): `floyd-go/` in this repo (Crush v0.92.0 rebrand, module
  `github.com/legacy-ai/floyd`, go 1.26). GitHub `CaptainPhantasy/MEGAFLOYD` is the
  single canonical remote. Never add or push to any charmbracelet upstream remote.
- Intelligence layer donor: `/Volumes/Storage/floyd` (v5.3 superfloyd line, branch
  v6-canonical). Read-only.
- Other donors checked and superseded: floyd-main, floyd-next worktrees, floyd-v5
  backups, .codex-tmp staging copies, DOUGLAS_MOVES/Floyd_Code-3.7, GO2TUI source-go-copy.

## Waves (sequential; each in its own worktree, gated by build+test)

Wave 1 — leaf/utility packages (19 files): paranoia, errors, fsops, fileops, pqueue,
validation, environment, intelligence, codebase/analyzer, agents/loader, ai, telemetry,
execution, db/embed, version/changelog, filetracker/write_tracker, permission/profile,
session/fork, skills/runtime.

Wave 2 — agent layer: internal/agent/{budget,context,prompt/cache,selector,summarizer,
swarm_coordinator}, ralph/, tools/ (21 tools + registry + mcp/health), workflow/,
plugins/, extensibility/.

Wave 3 — UI + commands: ui/dialog (6), ui/model (2), ui/terminal (2), ui/diffview,
cmd (11), app/lsp.go. UI work must follow internal/ui/AGENTS.md.

Wave 4 — integration + packaging: wire into app/backend/commands registry; `megafloyd`
naming (env MEGAFLOYD_*, home ~/.megafloyd, no CRUSH_/FLOYD_ command collisions);
non-Go assets (mcp-servers node bridges, extensibility assets, tool .md docs);
cloud install path (devcontainer/VM script); phone-reachable surface.

## Rules (binding)

1. Donor repos are read-only. All work lands in this repo.
2. No upstream connection: never fetch/pull/push charmbracelet/crush.
3. Never weaken permission checks; never commit secrets or credentials.
4. Every wave: `go build ./...`, `go test ./...` (race), `gofumpt -w .`, semantic
   one-line commits. Adapt donor code to chassis APIs — do not copy blindly.
5. Chassis conventions per `floyd-go/AGENTS.md` govern all edits.
6. Evidence over claims: a file is "ported" only when the wave gate passes.
