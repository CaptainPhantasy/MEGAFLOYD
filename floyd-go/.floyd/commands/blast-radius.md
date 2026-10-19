# Blast-Radius — Measure A Change's Reach

Before or after editing, compute how far a change can travel: which files depend on the touched symbols, how complex the affected code is, and which risks are real. The output is a tiered risk report, not a guess.

## Usage
```
/blast-radius [path-or-symbol]
```

## Procedure
1. Get the change surface: call `get_active_diff` for uncommitted work, or `git diff <base> -- <target>` via `bash` for a committed range.
2. Index the touched symbols: call `symbol_index` on the changed files; use `list_symbols` for a specific file's outline.
3. Find dependents: grep each touched exported symbol for call sites (the `grep` tool); map module relationships with `project_map`.
4. Score the impact: call `complexity_estimator` on the affected set — high-complexity dependents amplify risk.
5. Enumerate failure modes for the top risks: call `failure_mode_enumerator` on the change description.
6. Report tiers: DIRECT (files edited), REACH (files referencing edited symbols, with counts), RISK (top failure modes + complexity hotspots), and the verification order that covers the most risk first.

## Rules
- Every claim in the report cites its source (diff, symbol index, grep count) — no "probably unaffected".
- The verification order is the deliverable; a blast-radius report that does not change what gets tested first was not worth running.
