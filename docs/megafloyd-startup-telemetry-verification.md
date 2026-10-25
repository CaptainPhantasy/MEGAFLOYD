MegaFloyd login startup is enabled for the current Mac account. Its actual launchd run passed at 23:48:57 UTC on 24 October 2026: all six configured mounts were ready, the job exited 0, and the installed `234343` desktop remained the same process (`89919`) with no duplicate instance. The scheduled telemetry job also passed its actual launchd execution.

The amended `235658` release was checked read-only at 23:59:20 UTC on 24 October 2026. Its packaged startup helper matches the already installed internal helper exactly (`dae4ebd6…`); the current account’s mount configuration and LaunchAgent plist are unchanged. The same job still reports one run and last exit code 0. The actual six-mount/bootstrap proof remains the `234343` execution above; startup was not bootstrapped or run again for the amendment.

At 23:10:30 UTC on 24 October 2026, one authorized telemetry tick finished with exit code 0: it collected 1,174 events, including 1,172 new events, and shipped 1,172 events in three traces to the existing local Jaeger service. No model generation call was made.

The earlier scheduled attempt at 23:03:22 failed with exit code 126 because macOS denied the shell access to the external-drive launcher script. A single launchd probe then executed the installed native binary with `telemetry collect --help` and returned exit code 0. The tick script now uses this stable native path:

```
/Volumes/SanDisk1Tb/MEGAFLOYD/current/MEGAFLOYD.app/Contents/Resources/megafloyd/bin/megafloyd
```

It sets `MEGAFLOYD_ACTIVE=1` and `MEGAFLOYD_HOME=$HOME/.megafloyd`. The standard `HOME` variable and macOS permissions were not changed. The previous script was preserved beside it with a dated backup. The first probe receipt had an overly strict help-text matcher; the corrected receipt validates the already captured successful output without rerunning the probe.

The collector reads the current workspace’s own native session store and the canonical web store under `~/.megafloyd/web`. Its default discovery has no Floyd, F7, or Kimi home fallback. It does not automatically combine every native workspace. Its output remains under `~/.megafloyd/telemetry`, and shipping uses the existing loopback Jaeger endpoint.

The obsolete login item pointing to the Storage copy was removed and archived under `/Users/floyd/.megafloyd/startup/archive/20261024T220205186187Z`. Its application files were left untouched. DisplayLink Manager and Stats login items were preserved.

The replacement is one per-user LaunchAgent, `/Users/floyd/Library/LaunchAgents/ai.legacy.megafloyd.startup.plist`, with its helper and mount configuration in `/Users/floyd/.megafloyd/startup`. It opens the stable canonical application path through LaunchServices. It attempts all six configured work locations:

| Location | Mount path |
|---|---|
| SanDisk1Tb | `/Volumes/SanDisk1Tb` |
| Storage | `/Volumes/Storage` |
| applebottom | `/Volumes/applebottom` |
| STABLE | `/Volumes/STABLE` |
| STABLE - Data | `/Volumes/STABLE - Data` |
| inference SMB share | `/Volumes/floyd` |

The actual mount-only check at 22:01:33 UTC found all six already available and performed no mount or app launch. A focused retry check established that a missing optional share still allows the app to open, retries do not reopen it in the same login, recovery exits successfully, and a new login can open it again. Mount requests use saved system credentials without a credentials dialog. Each helper run is bounded to 50 seconds; launchd limits retries to one start every 30 seconds. Another OS account receives its own configuration only when it opts in.

The internal helper was refreshed from the installed `234343` package after preserving a dated backup. The one LaunchAgent was then bootstrapped successfully; its first actual run found all six locations already mounted, opened the existing canonical app through LaunchServices, and exited 0. The desktop stayed at PID `89919`, and the mount configuration and LaunchAgent plist remained byte-for-byte unchanged. This verifies actual execution in the current login; no reboot or forced unmount/remount test was performed.

The accompanying JSON retains exact paths, timestamps, prior failures, and the compact verification receipts.
