# megafloyd environment namespace

Status: VERIFIED against the chassis sources named below (2026-10-19).

`megafloyd` is a separate command with its own home directory. It does not
replace `floyd`, `superfloyd`, or `ff`, and it does not share their
configuration.

- Binary name: `megafloyd`
- Home: `~/.megafloyd` (override with `MEGAFLOYD_HOME`)
- Code: `floyd-go/internal/megafloyd`
- Launcher: `scripts/megafloyd`

## Canonical variables

These are the names megafloyd owns. Set them in the shell profile, in a
service unit, or let the launcher derive them.

| Variable | Default | Purpose |
|---|---|---|
| `MEGAFLOYD_HOME` | `~/.megafloyd` | Root of the layout |
| `MEGAFLOYD_CONFIG_DIR` | `$MEGAFLOYD_HOME/config` | Global `floydrc`, `floyd.json`, prompts |
| `MEGAFLOYD_DATA_DIR` | `$MEGAFLOYD_HOME/data` | Global data and the default `--data-dir` |
| `MEGAFLOYD_CACHE_DIR` | `$MEGAFLOYD_HOME/cache` | Regenerable caches, server logs |
| `MEGAFLOYD_IDENTITY` | unset | Agent identity name |
| `MEGAFLOYD_PROFILE` | unset | Non-empty enables the pprof endpoint |
| `MEGAFLOYD_SKILLS_DIR` | unset | Extra global Agent Skills directory |
| `MEGAFLOYD_ACTIVE` | unset | Set to `1` by the launcher |
| `MEGAFLOYD_POSTHOG_KEY` | unset | Opt-in telemetry; unset means telemetry stays off |

`MEGAFLOYD_STATE` is not a variable: private state always lives in
`$MEGAFLOYD_HOME/state`, next to config and data, so one `mv` or one backup
captures the whole installation.

## Compatibility mirrors

The chassis is a severed Crush fork and reads `FLOYD_*`. megafloyd exports the
mirrors below. They are the only variables the chassis needs to relocate.

| Chassis variable | Read at | Mirror of |
|---|---|---|
| `FLOYD_GLOBAL_CONFIG` | `internal/config/load.go:1216` | `MEGAFLOYD_CONFIG_DIR` |
| `FLOYD_GLOBAL_DATA` | `internal/config/load.go:1263` | `MEGAFLOYD_DATA_DIR` |
| `FLOYD_CACHE_DIR` | `internal/config/load.go:1239` | `MEGAFLOYD_CACHE_DIR` |
| `FLOYD_IDENTITY` | `internal/config/config.go:362` | `MEGAFLOYD_IDENTITY` |
| `FLOYD_PROFILE` | `main.go` (pprof) | `MEGAFLOYD_PROFILE` |
| `FLOYD_SKILLS_DIR` | `internal/config/load.go:1374` region | `MEGAFLOYD_SKILLS_DIR` |

Identity, profile, and skills are mirrored only when their `MEGAFLOYD_*`
counterpart is set. Directory mirrors are always exported.

## Precedence

For every value, the first non-empty wins:

1. Explicit `MEGAFLOYD_*` value.
2. An inherited `FLOYD_*` value (so an existing floyd environment keeps
   working when you switch commands).
3. The `~/.megafloyd` default.

`ApplyChassisEnv` leaves an inherited `FLOYD_*` variable untouched unless the
matching `MEGAFLOYD_*` variable is explicitly set; then the `MEGAFLOYD_*`
value wins. The launcher resolves the three directory values with the same
order before exporting the mirrors.

## Layout

```
~/.megafloyd/
  config/     global floydrc and floyd.json, prompts/
  data/       global data; default --data-dir
  cache/      caches and server logs
  state/      credentials and private state (mode 0700 recommended)
  logs/       launcher-visible logs
```

## Wiring

`main.go` calls `megafloyd.ApplyFromExecutable()` once, before `cmd.Execute`.
The hook acts only when the executable is named `megafloyd`; `floyd`,
`floyd-go`, and `go run` temporary binaries keep the chassis defaults.

The launcher is the supported entry point. It resolves the binary, exports
both namespaces, creates the layout, and passes `--data-dir` unless the caller
already supplied one. It ends in `exec`, so the binary owns the terminal and
its exit status.

## Adding a variable

1. Add the `MEGAFLOYD_*` constant to `internal/megafloyd/megafloyd.go`.
2. Mirror it in `ChassisEnv` only if the chassis already reads a `FLOYD_*`
   equivalent.
3. Document it in the table above.
4. Add a test in `internal/megafloyd/env_test.go`.

Do not add `MEGAFLOYD_*` reads inside `internal/config`, `internal/ui`, or
`internal/cmd`; the namespace belongs to this package and the launcher.

## Build

The binary is built by `scripts/build-megafloyd.sh` in the build workstream,
not by this package. Any build that writes `dist/megafloyd` is compatible with
the launcher.
