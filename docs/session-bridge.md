# Session bridge — desktop/web engine to CLI

The platform has two session stores:

| Surface | Store | Format |
|---|---|---|
| CLI (Go chassis) | `<data-dir>/floyd.db` | SQLite, UUID ids |
| Desktop / web engine | `$FLOYD_CODE_HOME` (default `~/.floyd-code`) | files: `session_index.jsonl`, `sessions/<scope>/<id>/state.json`, `agents/main/wire.jsonl` |

The bridge makes engine sessions visible to the CLI. It is **one-way and
read-only against the engine store**: nothing here writes to `~/.floyd-code`.

## Commands

```sh
# List sessions held by the engine (desktop/web).
megafloyd session engines
megafloyd session engines --json

# Copy an engine session into the local database.
megafloyd session import <engine-session-id>
megafloyd session import <engine-session-id> --json

# After import, the session appears locally and can be shown or resumed.
megafloyd session list
megafloyd session show <engine-session-id>
megafloyd -s <engine-session-id>
```

## Semantics

- The engine's session ID is kept as the local session ID, so imports are
  idempotent: importing twice reports “already imported” and copies nothing.
- Messages are reconstructed from `agent.message.appended` wire records:
  user/assistant text, reasoning (`think`), image URLs, and tool results.
  Tool call arguments are not reconstructed; the transcript shows results
  without the originating call.
- Engine home resolution: `FLOYD_CODE_HOME`, then `~/.floyd-code`, then
  `~/.megafloyd/web`. Only the first that contains `session_index.jsonl` is
  used.
- Malformed wire lines are skipped; a broken line never aborts the import.

## What this does not do

- It does not make the engine show CLI sessions. That direction still needs
  a platform-side import API.
- It does not keep the two copies in sync. Import is a one-time copy; new
  engine turns after the import are not reflected locally.
- A single authoritative store remains the long-term fix. This bridge is the
  compatibility layer until then, and it is recorded as PARTIAL in
  `specs.md` C6.
