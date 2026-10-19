# Why — Quote The Recorded Journal Entry

Answer "why did this happen?" from a job's append-only journal. The `why` tool quotes the entry with the exact action id and never invents a rationale.

## Usage
```
/why <JOB-ID> <ACTION-ID>
```

## Tool
`why` — parameters: `job_id` and `action_id`, both required.

## Procedure
1. Find a real action id with `replay` (full journal), `recover` (last three entries per unfinished job), or the recent-journal section of a `handoff` packet.
2. Call `why` with the exact `job_id` and `action_id`.
3. Quote what the tool returns: timestamp, tool, action, requirement, detail, artifact SHA-256, and result. The reply starts with `RECORDED` when the entry exists.
4. If the reply starts with `UNKNOWN`, say so. Do not reconstruct a rationale from surrounding code, commits, or memory.

## Rules
- Recorded history is the only evidence this tool accepts; inference is not an answer.
- Journal entries record tool actions: `contract created`, `requirement added`, `evidence recorded`, `completion recorded`, `handoff exported`.
- The journal is append-only; ids are stable and must be quoted exactly.
