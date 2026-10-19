# 11 — Context Window Protection

```md
Run this task with context-window discipline.

## Rules
- Prioritize high-signal outputs; no repeated restatements.
- Summarize completed steps every <N> actions.
- Keep one active objective at a time.
- If nearing limit: produce handoff block immediately.

## Handoff block template
- Current objective
- Completed evidence
- Remaining steps
- Exact next command/action
- Known risks

## Gate
Never continue blindly when context pressure is high.
```