# findings.md — MEGAFLOYD reference (for the next model)

> Author note: curated so a fresh session can resume without re-discovery.
> Sources: `docs/MERGE-STATUS-20261016.md`, `docs/prompt-pack/MANIFEST.md`, `PLAN.md`,
> and independent verification run 2026-10-16.

## A. What MEGAFLOYD is

Work-in-progress merger of two Floyd lines into one build, at `/Users/floyd/MEGAFLOYD`
(`origin=github.com/CaptainPhantasy/MEGAFLOYD`), commit `e33af3f`. **Not wired to any
existing install.** Scratch project.

| Path | What | Origin |
|---|---|---|
| `floyd-code/` | Floyd Code v2.0.2, TS/pnpm monorepo | copy of `floyd-v6/KimiSource/kimi-code` (`.git`/`node_modules`/dists excluded) |
| `floyd-go/` | Go agent behaviour layer | copy of `floyd-v6/src` |
| `docs/prompt-pack/` | Go prompt templates + assembly ref, as plain data | `floyd-go/internal/agent/{templates,prompt}` |
| `PLAN.md` | phases P0–P5, plug points, gates, risks | pre-copy |
| `docs/MERGE-STATUS-20261016.md` | point-in-time audit of P0/P1 | this tree |
| `docs/prompt-pack/MANIFEST.md` | canonical pack interface + checksums | this tree |
| `scripts/verify-prompt-pack.mjs` | drift guard | this tree |

## B. Verified state (2026-10-16, re-checked)

| Claim | Result |
|---|---|
| `floyd-code/` = copy of kimi-code, exclusions only | TRUE |
| Floyd Code version 2.0.2 | TRUE (`apps/floyd-code/package.json`) |
| `floyd-go/` = copy of `floyd-v6/src`, one file differs | TRUE (`internal/event/event.go`) |
| `docs/prompt-pack/` byte-identical to source | TRUE |
| `scripts/verify-prompt-pack.mjs` | **PASS** (derived copy + unversioned copy both match canonical) |
| P0 build proof reproducible | **Improved** — `floyd-code/node_modules` and `apps/floyd-code/dist` are now PRESENT (MERGE-STATUS F5 said absent); still unverified end-to-end this session |

### Toolchain (measured 2026-10-16)

| Tool | Local | Required |
|---|---|---|
| node | v26.9.0 | `>=24.15.0` ✓ |
| pnpm | **10.25.0** | `pnpm@10.33.0` (packageManager) — mismatch, corepack fetches 10.33.0 |
| go | **go1.25.7** | `go.mod` says `go 1.26.6` → GOTOOLCHAIN download on first `go build` |

## C. The prompt-pack truth (F3, resolved)

- **Canonical:** `docs/prompt-pack/templates/` (8 files). Edit here only.
- **Derived (physical copy required by `//go:embed`):** `floyd-go/internal/agent/templates/`.
  Never edit directly. Sync with `node scripts/verify-prompt-pack.mjs --sync`.
- **Advisory (outside git):** `../floyd-merge/prompt-pack/templates/` — matches; not authoritative.
- The Go assembler's real interface is the `PromptDat` struct (`prompt.go:31`), **not** the
  variable names listed in `PLAN.md` P1. See `MANIFEST.md` §2.
- Only 3 templates (`coder`, `task`, `initialize`) are overridable from
  `<global config dir>/prompts/<name>.md.tpl`; the other 5 are embedded-only.

## D. Plug-point paths (F2, corrected)

| Task | Actual file(s) |
|---|---|
| Prompt/profile injection | `packages/agent-core-v2/src/app/agentProfileCatalog/{agentProfileCatalog.ts,profile-shared.ts}` |
| System/context files | `packages/agent-core-v2/src/workspace/workspaceAgentProfileLoader/internal/systemFile.ts`, `.../workspaceAgentProfileLoader/userAgentProfileLoaderService.ts` |
| Workspace instructions | `packages/agent-core-v2/src/workspace/workspaceInstructions/workspaceInstructionsService.ts`; `.../agent/agentsMdReminder/agentsMdReminderService.ts` |
| Session bootstrap | `packages/agent-core-v2/src/features/sessionInit/` |
| Tool discipline rules | `packages/agent-core-v2/src/features/externalHooks/agent/agentExternalHooksService.ts`; `.../externalHooks/internal/types.ts` |
| Compaction/summary | `packages/agent-core-v2/src/agent/fullCompaction/fullCompactionService.ts` (+ `compactionInstruction.ts`) |

All 11 paths verified present 2026-10-16.

## E. Secret / telemetry (F1)

- Merged `floyd-go/internal/event/event.go` reads `MEGAFLOYD_POSTHOG_KEY` from env; no key baked in. ✓
- `.gitleaks.toml` allowlists 4 reviewed paths only.
- **Open gap:** the hardcoded PostHog key `phc_4zt4…I0V` still lives in the SOURCE repo
  `floyd-v6/src/internal/event/event.go:18` (outside this project). Not fixable from here without
  touching a source repo. Recorded, not repaired.

## F. Housekeeping (F6)

- `floyd-go/.envrc`, `floyd-go/.github/`, `.gitattributes` copied in from source — dead config.
- `floyd-code/services/company-platform/` (Python + JS + MJS) — needs an explicit in/out decision
  before P5's egress review.

## G. Do-not-cache

- Tool counts, `reachable` flags, build-output presence (`node_modules`, `dist`) — all change
  between sessions. Re-measure.

## H. Session-2 defect inventory — "required artifact invisible to git" (VERIFIED)

Class: an over-broad, **unanchored** `.gitignore` rule (a bare directory/file name) matches a
real source path at any depth, so the file exists locally but is never committed — and is then
absent from every clone. One copy-time exclusion is in the same class. Six instances total.

| # | Artifact | Rule that hid it | State |
|---|---|---|---|
| 1 | `floyd-code/build/{raw-text-plugin,raw-text-loader,register-raw-text-loader}.mjs` | root `.gitignore` `build/` | fixed `9088cd4` |
| 2 | `apps/vis/web/src/components/logs/LogsTab.tsx` | root `.gitignore` `logs/` | fixed `f8c4541` |
| 3 | `packages/agent-core-v2/src/human/{test/,}credentials/*.ts` (5) | root `.gitignore` `credentials` | fixed `f8c4541` |
| 4 | `packages/migration-legacy/test/fixtures/{golden,multi-workdir}/.floyd/**` (26) | root `.gitignore` `.floyd/` | fixed `f8c4541` |
| 5 | `packages/klient/Dockerfile` | `floyd-code/.gitignore:26` bare `Dockerfile` | **open** |
| 6 | `apps/floyd-code/dist-web/**` (524) | copy exclusion (`.git/…/dist-native/dist-web` dropped) + root `dist-web/` | **open** |

Fix applied (`f8c4541`): a trailing **negation block** in the root `.gitignore` — explicit `!`
re-includes placed **after** the excluding patterns (last-match-wins). Negative controls confirm
`node_modules/`, `dist/`, `.floyd/`, `build/` at foreign depths stay ignored.

Sweep proof (VERIFIED 2026-10-16):
`comm -23 <(git -C <source> ls-files | sort) <(git -C <mega> ls-files -- floyd-code | sed 's#^floyd-code/##' | sort)`
→ **525 paths = 524 × dist-web + 1 × klient/Dockerfile**. No other omissions.

`dist-web` is a **committed, prebuilt** bundle (developed in the upstream `code-app` repo,
shipped here per `floyd-code/AGENTS.md` as "gitignored, force-added"). `check-web-assets.mjs`
makes packaging fail loudly if it is absent — correct behavior; the tree simply lost the file.

## I. Infrastructure leads (UNVERIFIED — do not design against until confirmed)

- Owner note 2026-10-16: the plugin/marketplace **CDN is "maybe" on Cloudflare**; the **admin
  console is hosted by "indeeence"** (owner's spelling; identity unconfirmed). Both are `LEAD`,
  not fact. Confirm via a run (DNS/HTTP, or the marketplace's catalog fetch URL) before use.

## J. Toolchain reality check (2026-10-16)

- Mac (client-only): node v26.9.0 both trees; **pnpm 10.33.0** on the SanDisk1Tb clone.
- Inference host: Ubuntu 7.0.0-38, reachable at `100.78.109.105`, login `LEGACYAI\floyd`; **no
  docker/podman** — plain node processes + systemd. MCP secret-broker runs there (port 9717).
