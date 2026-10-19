# Drift — Detect Plan-Reality Divergence

Compare what the plan/SSOT says against what the code and state actually are, and list every place they disagree. Drift is detected, reported, and then either the state or the doc is corrected — never left ambiguous.

## Usage
```
/drift [doc-path]
```

## Procedure
1. Load the reference: the SSOT.md section or plan file given (default: the active plan in `.harness/plans/` or the repo SSOT).
2. Extract checkable claims: statuses ("done", "verified"), file lists, behavior statements, command names.
3. Verify each against reality: `grep`/`glob` for files and symbols, `get_active_diff` and `git log` for claimed work, `bash` to run claimed-passing checks. Tag each claim VERIFIED or DRIFTED (SSOT P1 evidence tags).
4. Report the drift table: claim | source line | reality | evidence. Group into: doc behind code (doc needs update), code behind doc (work missing), claims without evidence (need verification or removal).
5. Propose the realignment per row; apply doc-side fixes immediately with the `edit` tool (code-side fixes are tasks, not edits here — /plan them).

## Rules
- Every DRIFTED row cites both sides: the doc line and the reality check that contradicts it.
- Drift runs read-only against code; it never "fixes" reality by editing the doc to match broken behavior — that is a decision for the caller.
