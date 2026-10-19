# 08 — Release Readiness Audit

```md
Audit this branch for release readiness.

## Audit checklist
- Build/test/lint status
- Risky diffs
- Backward compatibility
- Migration/data impact
- Observability/logging impact
- Rollback path

## Required output
- GO / NO-GO decision
- Top 5 release blockers (if any)
- Minimal unblock plan
- Test plan checklist

## Completion gate
No GO without explicit evidence for each checklist item.
```