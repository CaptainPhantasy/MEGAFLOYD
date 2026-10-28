# Learn — Crystallize A Lesson Into Reusable Skill

After a task reveals a repeatable insight, capture it where future sessions will find it: a task procedure or project fact in the project's recorded locations. Learning that is not written down did not happen.

## Usage
```
/learn <LESSON-DESCRIPTION>
```

## Procedure
1. Distill: one sentence each — the situation, the insight, the action it should trigger. Drop the narrative; keep the pattern.
2. Choose the home:
   - Reusable procedure (a trigger + steps): write a command file with the `write` tool to `<selected-project-data-directory>/commands/<name>.md`, resolving the selected project data directory from the running harness configuration, using the actual command loader's format. Record its trigger and technical steps; do not introduce another behavioral policy.
   - Project facts or user decisions: update the project's `<ProjectName>_SSOT.md` in its recorded document folder, preserving the source and evidence.
   - A requested behavioral policy change belongs in MegaFloyd.md. Amend it only when the user explicitly requested that policy change; an inferred lesson does not authorize it.
3. Keep it tight: a skill file over ~40 lines will not be read under pressure. Name the exact tools/commands, verified by grep, not from memory.
4. Report where the lesson landed and the trigger that will recall it.

## Rules
- One lesson, one home — do not write the same insight into three places.
- Lessons that contradict owner directives are surfaced to the owner, not written as rules.
