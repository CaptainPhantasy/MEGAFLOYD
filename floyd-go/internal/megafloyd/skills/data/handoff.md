# Handoff — Portable Job Packet

Package one job so a fresh session can continue without rediscovering state. The `handoff` tool writes the packet and returns it in full.

## Usage
```
/handoff <JOB-ID>
```

## Tool
`handoff` — parameter: `job_id` (required). Writes `jobs/<job_id>/handoff.md` under the data directory.

## Procedure
1. Call `handoff` with the job id.
2. Read the returned packet before any other tool call:
   - objective, status, artifact path, and current and created SHA-256;
   - declared scope;
   - per-requirement verdicts, reasons, and latest evidence;
   - the exact next action;
   - the last ten journal entries;
   - continuation paths relative to the job directory.
3. Hand the packet path to the successor. It is self-contained; excerpt paths are relative to the job directory.

## Rules
- The tool owns the packet. Do not hand-write `continue.md` or a second summary that can drift from the job record.
- Evidence over adjectives: the packet quotes exit codes and hashes, not "tests pass".
- Write a handoff before a long pause or context reset, not after the context is gone.
- A handoff does not re-run checks; the next action still needs fresh `prove` evidence when the artifact has changed.
