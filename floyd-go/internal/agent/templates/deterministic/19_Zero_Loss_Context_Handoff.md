# 19 — Zero-Loss Context Handoff

```md
Use before context exhaustion or when switching sessions/operators.

## Required handoff package
1) Current objective
2) Exact completed actions + evidence
3) Unfinished actions in order
4) Current branch/status/tests state
5) Known risks + immediate next command

## Strict formatting
- Evidence references must be concrete (path, command, output snippet)
- Next action must be executable without interpretation

## Restart prompt
"Resume objective: <objective>. Current state: <state>. Execute next action: <exact action>."

## Gate
If next action is ambiguous, handoff is invalid.
```
