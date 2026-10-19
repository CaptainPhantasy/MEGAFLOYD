# Learn — Crystallize A Lesson Into Reusable Skill

After a task reveals a repeatable insight, capture it where future sessions will find it: a tight skill/command file or a line in the project guidance. Learning that is not written down did not happen.

## Usage
```
/learn <LESSON-DESCRIPTION>
```

## Procedure
1. Distill: one sentence each — the situation, the insight, the action it should trigger. Drop the narrative; keep the pattern.
2. Choose the home:
   - Reusable procedure (a trigger + steps): write a command file with the `write` tool to `.floyd/commands/<name>.md` in this repo's format (title, usage, procedure, rules) — it loads as a user command (internal/commands).
   - Project-specific guidance: add a line to AGENTS.md (or `.agents/skills/` guidance) with the `edit` tool.
   - Cross-project habit already covered by an existing skill: amend that skill's Rules section instead of creating a near-duplicate.
3. Keep it tight: a skill file over ~40 lines will not be read under pressure. Name the exact tools/commands, verified by grep, not from memory.
4. Report where the lesson landed and the trigger that will recall it.

## Rules
- One lesson, one home — do not write the same insight into three places.
- Lessons that contradict owner directives are surfaced to the owner, not written as rules.
