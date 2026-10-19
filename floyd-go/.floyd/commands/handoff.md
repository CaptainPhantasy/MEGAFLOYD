# Handoff — Cross-Session Continuation Package

Package the current session's state so a fresh session (or a fresh agent tomorrow) can continue exactly where this one stopped, without rediscovering anything.

## Usage
```
/handoff
```

## Procedure
1. Export the full conversation with `transcript_export` — the raw record the next session can grep.
2. Fork the session at the latest message (session fork mechanism, internal/session/fork.go) so the successor can resume with full context loaded.
3. Write `continue.md` in the working directory (or the active worktree root) with the `write` tool:
   - Objective: the task in one sentence.
   - Done: verified completions, each with its evidence (command run + result).
   - Not done: remaining work, exact next action first.
   - Locked values: names, paths, ids, amounts that must not change.
   - Landmines: known blockers, concurrent-agent boundaries, files not to touch.
   - Continuation point: the forked session id and the transcript path.
4. Pull open items from the `todos` tool into the Not done list.
5. Reply with three paths: continue.md, transcript, forked session id.

## Rules
- Evidence over adjectives: "tests pass" is not handoff content; "go test ./internal/config exit 0" is.
- The successor reads continue.md before any tool call — write it for a cold reader.
