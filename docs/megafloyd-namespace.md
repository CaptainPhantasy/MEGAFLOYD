# MegaFloyd environment namespace

Verified against the canonical source and installed release `megafloyd-20261024T234343Z` on 2026-10-24. The implementation source commit is `8c4876a3d166d9ac1ddd9952f27b2ab0bed3dfd3`; this documentation update does not rebuild that release.

MegaFloyd uses the calling OS account's own home and workspace stores. The supported installed entrypoints are `<prefix>/bin/megafloyd`, its `megafloid` spelling alias, and `<prefix>/MEGAFLOYD.app`. The installer selects one immutable release through `<prefix>/current`. See [release packages](megafloyd-release.md) and [installed verification](megafloyd-release-verification.md).

## Canonical variables

| Variable | Default | Purpose |
|---|---|---|
| `MEGAFLOYD_HOME` | `~/.megafloyd` | The calling account's application home |
| `MEGAFLOYD_CONFIG_DIR` | `$MEGAFLOYD_HOME/config` | Global user configuration and prompts |
| `MEGAFLOYD_DATA_DIR` | `$MEGAFLOYD_HOME/data` | Global settings and the parent of default private workspace stores |
| `MEGAFLOYD_CACHE_DIR` | `$MEGAFLOYD_HOME/cache` | Provider metadata, regenerable caches and server logs |
| `MEGAFLOYD_WEB_DATA` | `$MEGAFLOYD_HOME/web` | Web adapter state |
| `MEGAFLOYD_IDENTITY` | `MegaFloyd` | Agent identity label |
| `MEGAFLOYD_PROFILE` | unset | Non-empty enables the pprof profile endpoint |
| `MEGAFLOYD_SKILLS_DIR` | unset | Extra global Agent Skills directory |
| `MEGAFLOYD_ACTIVE` | set to `1` by the launcher | Marks the canonical runtime |
| `MEGAFLOYD_POSTHOG_KEY` | unset | Opt-in telemetry key |

For application paths, an explicit `MEGAFLOYD_*` value wins; otherwise the path is derived from the caller's MegaFloyd home. Inherited `FLOYD_GLOBAL_CONFIG`, `FLOYD_GLOBAL_DATA`, `FLOYD_CACHE_DIR` and `FLOYD_CODE_HOME` do not select another application's state. The standard OS `HOME` is not reassigned.

Internal `FLOYD_*` directory mirrors are derived from the resolved MegaFloyd paths for chassis code that still reads those names. They are implementation inputs, not permission to discover a donor application's home. Identity, profiling and skills mirrors are supplied when their corresponding canonical overrides are set; canonical identity also has its own default.

## Workspace state and configuration

An explicit `--data-dir` selects the workspace store. Otherwise the native runtime resolves the workspace's absolute real path, checks this account's `state/workspace-data.json` for an explicit mapping, and defaults to:

```
$MEGAFLOYD_DATA_DIR/workspaces/<sha256-of-canonical-workspace-path>/
```

New workspace store directories are created with mode `0700`. Two different account homes opening the same project therefore get separate session stores. A corrupt or unreadable private mapping produces an error instead of silently selecting a different store. The web adapter uses the directory returned by Go; it does not impose a project `.megafloyd` default.

The current owner's explicitly mapped existing verification store remains at `/Volumes/SanDisk1Tb/MEGAFLOYD/.tmp/intelligence-verification/workspace/.megafloyd`. That deliberate mapping preserves its history. It is not a default for a fresh account or an invitation to import another application's database.

Canonical project configuration names are `.megafloydrc`, `megafloydrc`, `.megafloyd.json` and `megafloyd.json`, searched within the current project boundary. The system path is `/etc/megafloyd/megafloyd.json`. Global files retain the native filenames `config/floyd.json` and `data/floyd.json` inside the MegaFloyd home; the user config directory can also contribute its shell config sibling. Those basenames do not imply reads from `~/.floyd` or another application's configuration directory.

## Layout

```
~/.megafloyd/
  config/                 global user configuration and prompts
  data/floyd.json         global saved settings
  data/workspaces/<hash>/ default private workspace stores
  cache/                  provider metadata, caches and server logs
  state/workspace-data.json  explicit account-owned workspace mappings
  credentials/            native company account credential slot
  web/                    web adapter state
  desktop/                Electron profile and remembered navigation
  logs/                   application logs
```

Memories, scratchpad, checkpoints, jobs and exports follow the resolved personal workspace store. The home contains per-user startup settings only when that user explicitly enables startup; another account's credentials, volume UUIDs and network-share choices are not packaged or copied. Explicit configuration/cache/data/web overrides can place those directories outside the default layout and must be included separately in the owner's backup plan.

## Source references

- [Directory resolution](../floyd-go/internal/megafloyd/dirs.go) and [canonical activation](../floyd-go/internal/megafloyd/runtime.go).
- [Private workspace resolution](../floyd-go/internal/megafloyd/workspace_data.go) and [personal-state selection](../floyd-go/internal/megafloyd/personal_data.go).
- [Configuration loading](../floyd-go/internal/config/load.go), [environment mirrors](../floyd-go/internal/megafloyd/env.go) and [web home](../floyd-go/internal/megafloydweb/home.go).
- [Source launcher](../scripts/megafloyd) and [installed release launcher generation](../scripts/megafloyd-release.py).

The source launcher passes explicit arguments unchanged and does not add a default `--data-dir`. The installed launcher resolves `current` once and runs the native executable embedded in that release, with its matching web and desktop resources.
