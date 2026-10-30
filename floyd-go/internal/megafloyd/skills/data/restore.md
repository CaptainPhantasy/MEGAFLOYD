# Restore — Return To A Checkpoint

Bring work back to a state saved by /checkpoint: reopen the recorded session position, reset git state, and resume from the recorded next step. The inverse of /checkpoint.

## Usage
```
/restore <NAME>
```

## Procedure
1. Read `<data-directory>/checkpoints/<name>.md` with the `view` tool. If the name is ambiguous, list that checkpoints directory and match by timestamp.
2. Restore git state: `git checkout <recorded-branch>` then `git reset --hard <recorded-HEAD>` only after confirming with the caller that post-checkpoint commits may be discarded; otherwise `git stash` current work first. Restore the stashed dirty files if a stash ref is recorded.
3. Restore conversation position: reopen the forked checkpoint session via `megafloyd session show <id>` (or switch to it in the TUI session picker). If no fork was recorded, resume the recorded session id.
4. Reload the open todos recorded in the checkpoint into the `todos` tool.
5. State plainly: restored checkpoint name, what was discarded (commits/dirty files/conversation tail), and the recorded next step. Then execute the next step.

## Rules
- Destructive git resets need explicit confirmation unless the checkpoint was taken seconds ago in the same task.
- Never delete the checkpoint file on restore — it stays as history.
