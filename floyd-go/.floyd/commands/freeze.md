# Freeze — Lock Permissions Down

Snapshot the current permission grants, then restrict the session to a minimal safe profile: read-only tools allowed, writes and shell blocked. Use before letting a risky task, untrusted content, or a long unattended run proceed. The inverse is /thaw.

## Usage
```
/freeze [reason]
```

## Procedure
1. Snapshot: record the current grants — the permission allow/deny lists from active config and the profile grants file (`permissions.json`, loaded by internal/permission/profile.go: `LoadProfiles`). Write the snapshot with the `write` tool to `.floyd/profiles/freeze-snapshot-<timestamp>.json`.
2. Apply the restricted profile: write a minimal grant set to the profile store (only view/grep/glob/ls-style reads; no bash, no edit, no write) — internal/permission/profile.go: `SaveProfile` is the mechanism the profile store uses.
3. Verify the lockdown: attempt a harmless write with the `bash` tool (e.g. `touch .floyd/freeze-probe`); it must be denied. Report the denial as evidence.
4. State the freeze plainly: what is snapshotted where, what remains allowed, and that /thaw restores.

## Rules
- Never weaken permission checks to make a freeze "take" — the freeze works because denials work.
- A freeze without a verified denial is not a freeze; the probe in step 3 is mandatory.
