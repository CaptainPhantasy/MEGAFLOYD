# Thaw — Restore Permissions After A Freeze

Reverse a /freeze: restore the permission grants from the freeze snapshot, verify writes work again, and report the restored state. The inverse of /freeze.

## Usage
```
/thaw
```

## Procedure
1. Find the newest snapshot: `.floyd/profiles/freeze-snapshot-<timestamp>.json` (the `glob` tool finds them; newest by timestamp in the name).
2. Restore the grants from the snapshot into the profile store (internal/permission/profile.go: `SaveProfile`) and the active config's allow lists.
3. Verify the thaw: attempt the same probe that freeze used (`touch .floyd/thaw-probe` via `bash`) — it must succeed now; then remove both probe files.
4. Report: snapshot restored, freeze lifted at <timestamp>, permission state back to pre-freeze. If no snapshot exists, say so plainly and do not improvise a grant set.

## Rules
- Thaw restores exactly what freeze snapshotted — never use thaw to quietly grant more than the session had before.
- If the session was frozen for a security reason (redteam finding, injected content), name that reason and confirm it is resolved before thawing.
