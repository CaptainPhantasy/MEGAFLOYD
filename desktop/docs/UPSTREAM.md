# Upstream watch — adopting Kimi desktop changes

MEGAFLOYD Desktop is a fork of the Kimi desktop shell. Kimi’s desktop is
closed source and has **no official GitHub release feed**. The GitHub hits
for “Kimi desktop” are community mirrors and different products (Kimi Code,
Tauri wrappers); they are signals, not authority.

There is **no automatic update path**. Nothing downloads, installs, or
executes upstream code without a human decision. This document is that
decision process.

## What is pinned

| Item | Where | Current |
|---|---|---|
| Kimi base app | `vendor/Kimi-<version>-arm64.app` | 3.2.17 (build 3.2.17.4, Electron 43.7.3) |
| Platform commit | `PLATFORM-COMMIT` | `bd08789` |
| Shell version | `desktop/package.json` | 1.0.0 |

Everything the fork changes is recorded in `docs/REBRAND.md`.

## Checking for upstream movement

Run from the repo root:

```sh
make upstream
```

or directly:

```sh
node desktop/scripts/check-upstream.mjs
```

It reports:

- the pinned vendor version (from the app’s `Info.plist`),
- the newest release tag of a community mirror (`Kimi Code` — a different
  product; a hint only),
- optionally, a version found on an official download page when
  `KIMI_DOWNLOAD_URL` is set.

Exit `0` = no newer signal. Exit `10` = a newer signal was found; decide
whether to adopt. Exit `2` = the check could not run.

The script never downloads or executes anything.

## Deciding what to take

Upstream changes are not merged wholesale. For each release, list what it
contains and mark it:

| Date | Upstream version | Change | Take? | Why | Adopted in |
|---|---|---|---|---|---|
| 2026-10-19 | 3.2.17 | initial base | yes | fork starting point | `c912a00` |

Rules:

- Take only what serves MEGAFLOYD. Kimi cloud, updater, telemetry, and
  identity stay out (see `docs/REBRAND.md`).
- If a change touches the shell surface (windows, menus, tray, deep links,
  IPC), it must be re-applied to `desktop/src`, not copied over it.
- If a change is in the engine/web UI, it belongs to the platform repo, not
  the desktop; port it there first, then re-pin.
- Record rejected changes too, with the reason, so they are not re-debated.

## Adopting a new base (manual, gated)

1. Download the official DMG for macOS arm64. Do not use a mirror binary.
2. Verify the bundle: `CFBundleIdentifier` must be `com.moonshot.kimichat`
   (changed later by our build), version must match the release.
3. Mount read-only and replace `vendor/Kimi-<old>-arm64.app` with the new
   app. Never modify the vendor app in place.
4. Re-apply or adjust the rebrand in `desktop/scripts/build.mjs` only where
   the new base differs (helper names, resources, plists, updater files).
5. Update the table in this file and any stale version references.
6. Rebuild and prove it:
   ```sh
   make build
   make verify
   make smoke
   make smoke-ui
   ```
7. Only after all four pass: commit the new vendor pin (the vendor app is
   not committed; commit the version references and rebrand changes) and
   record the adoption in `docs/REBRAND.md`.

## What never comes back

- Kimi’s update feed, cloud gateway, telemetry, localization packs.
- Kimi identity, bundle id, helper names, deep-link scheme.
- Any remote surface: the engine binds loopback; Tailscale remains an engine
  feature, not a desktop one.

## Known gaps

- No public changelog for the desktop app; the official download page is
  the only authoritative signal.
- The community mirror tracked by the script publishes Windows binaries of
  Kimi Code, not the desktop; treat it as evidence that upstream is active,
  never as a version to adopt.
