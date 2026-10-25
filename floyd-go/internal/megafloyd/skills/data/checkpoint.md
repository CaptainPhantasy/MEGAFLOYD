# Checkpoint — Save A Return Point

Record a named, restorable snapshot of the current state: conversation position, git commit, dirty files, and the next step. Cheap insurance before a risky change. Restoring is /restore.

## Usage
```
/checkpoint <NAME>
```

## Procedure
1. Record git state: current branch, `git rev-parse HEAD`, and `git stash create` (or note the dirty diff) so uncommitted work is capturable.
2. Capture the active diff with the `get_active_diff` tool.
3. Write the checkpoint record with the `write` tool to `.harness/checkpoints/<name>.md` containing:
   - name and timestamp
   - session id (`megafloyd session list` shows it) and current message position
   - git branch, HEAD commit, dirty-file list, stash ref if created
   - open todos (from the `todos` tool)
   - the intended next step, one sentence
4. For a conversation-state checkpoint, also fork the session at the current message (session fork mechanism, internal/session/fork.go) so the exact conversation position is preserved as a session.
5. Reply with the checkpoint path and the session id. One line.

## Rules
- Checkpoints are additive — never delete or overwrite another checkpoint; a re-checkpoint under the same name gets a `-2` suffix.
- Keep the record file small: facts only.
