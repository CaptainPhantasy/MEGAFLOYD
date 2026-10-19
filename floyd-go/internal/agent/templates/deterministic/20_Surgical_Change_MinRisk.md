# 20 — Surgical Change (Min-Risk)

```md
Use for high-risk areas where only smallest safe change is acceptable.

## Target
<file/component>

## Rules
- Smallest possible diff to solve symptom
- No unrelated edits
- Keep rollback trivial

## Required flow
1) Pre-change behavior snapshot
2) Minimal patch
3) Post-change verification
4) Risk statement + rollback command

## Output
- Patch intent (1 sentence)
- Exact changed lines/files
- Verification proof
- Rollback instructions

## Gate
If patch scope expands beyond symptom boundary, stop and re-approve scope.
```
