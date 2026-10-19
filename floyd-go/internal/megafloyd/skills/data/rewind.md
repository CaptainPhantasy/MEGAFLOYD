# Rewind — Roll Conversation Back To An Earlier Point

Undo the recent conversation by forking the session at an earlier message. Everything after that point is dropped from the active thread but preserved in the old branch. Use when the current approach went wrong and the good state was a few messages ago.

## Usage
```
/rewind <MESSAGE-NUMBER-OR-DESCRIPTION>
```

## Procedure
1. Identify the target point: the message index or a description of the last good exchange. Use `transcript_export` if the numbering is unclear.
2. Fork the session at that message using the session fork mechanism (internal/session/fork.go: `Fork(sourceSessionID, upToMessage)` — exposed in the TUI as the built-in fork action). The new session contains history up to and including that message.
3. Continue work in the forked session from the rewound state. Do not repeat the failed path — state in one line what went wrong after the rewind point so it is not retried.
4. The pre-rewind branch remains intact; reference it with `floyd session list` / `floyd session show <id>` if the dropped content is needed.

## Rules
- Rewind is for conversation state. To also undo file changes, pair with git (`git stash`, `git checkout`) or /checkpoint-restore — say plainly which of the two you are rewinding.
