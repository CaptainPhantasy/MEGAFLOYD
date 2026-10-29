# Retro — Retrospective On The Work Just Done

Review the completed task honestly: what happened, what worked, what failed, what to change. Produces a short written record and feeds /learn.

## Usage
```
/retro [SESSION-OR-TASK]
```

## Procedure
1. Reconstruct the timeline: `transcript_export` the session; skim git log for the task's commits. Facts before feelings.
2. Answer four questions, each with evidence from the timeline:
   - What worked (and should be repeated)?
   - What failed or cost more than it should (with the root cause, not the symptom)?
   - What surprised us (unknown unknowns that became known)?
   - What will we do differently next time (one concrete change)?
3. Write the record to the SSOT's recorded evidence folder (`docs/evidence/retros/<date>-<slug>.md` when none is recorded) with the `write` tool — half a page maximum.
4. Feed the durable insights into /learn; the retro file keeps the story, the skill keeps the pattern.
5. Report the four answers compactly. No ceremony, no blame — mechanism talk only.

## Rules
- A retro without at least one "do differently" item was not honest; a retro with ten was not focused.
- Never let a retro item live only in the file: each becomes a /learn entry, a todo, or a deliberate drop (stated).
