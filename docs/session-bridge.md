# Session bridge — CLI and desktop/web engine

The platform has two session stores:

| Surface | Store | Format |
|---|---|---|
| CLI (Go chassis) | `<data-dir>/floyd.db` | SQLite, UUID ids |
| Desktop / web engine | `$FLOYD_CODE_HOME` (default `~/.floyd-code`) | files: `session_index.jsonl`, `sessions/<scope>/<id>/state.json`, `agents/main/wire.jsonl` |

The bridge makes each store's sessions visible to the other. Import reads the
engine store and never writes to it; export writes only to an engine home the
caller names explicitly.

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

# Write a local session into an engine home so the desktop/web lists it.
megafloyd session export <id> --engine-home /path/to/engine-home
megafloyd session export <id> --engine-home /path/to/engine-home --json
```

`--engine-home` has no default. Export refuses to guess, because writing into
`~/.floyd-code` by accident would pollute the live engine store.

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
- Export writes the engine's own event stream: `step.begin`, `content.part`
  (`stepUuid` on every part), `tool.call`, `tool.result`, `prompt.completed`,
  `turn.ended`, `step.end`. The engine derives turns, steps, user and
  assistant messages from those records, so a transcript opened in the
  desktop shows the same turns the CLI has.
- A rewritten session under the same ID replaces its `state.json` and
  `wire.jsonl` in place. The index row is upserted, not duplicated.

## What this does not do

- It does not keep the two copies in sync. Import and export are one-time
  copies; later turns in either store are not reflected in the other.
- A single authoritative store remains the long-term fix. This bridge is the
  compatibility layer until then, and it is recorded as PARTIAL in
  `specs.md` C6.
