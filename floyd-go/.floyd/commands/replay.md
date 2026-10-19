# Replay — Re-Run A Recorded Sequence

Re-execute a previously recorded run (reproduction, session steps, benchmark) against the current state and compare outcomes to the original. Regression detection by literal repetition.

## Usage
```
/replay <RECORD-REF>
```

## Procedure
1. Load the record: a repro script from `.harness/repro/` (see /reproduce), a transcript via `transcript_export`, or a recorded session via `floyd session show <id>`.
2. Extract the steps in order: commands run, inputs given, environment pinned. Steps that no longer exist (renamed files, dead endpoints) are listed as BROKEN before starting.
3. Re-execute faithfully via `bash` — same commands, same order, same flags. Independent steps may batch through `parallel_bash` only when the record shows they were independent.
4. Capture each step's outcome; diff against the recorded outcomes (exit codes, key output lines).
5. Report: steps replayed, outcomes MATCHED / CHANGED (with the delta) / BROKEN. CHANGED outcomes are leads — tie each to /debug or /why for the cause.

## Rules
- Replay does not "fix" steps that fail — fidelity is the method. Adaptation happens after reporting, explicitly.
- Deep record/replay plumbing (cradle-to-grave step capture) is future Go work; today's records are repro scripts and transcripts, and this skill executes exactly those.
