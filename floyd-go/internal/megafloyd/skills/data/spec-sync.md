# Spec-Sync — Reconcile Spec And Code

Bring the spec (SSOT, plan, API docs) and the implementation into agreement after changes landed: verify each spec statement against code, update whichever side is stale, and leave evidence tags. Heavier than /drift (which detects); spec-sync reconciles both directions.

## Usage
```
/spec-sync [spec-path]
```

## Procedure
1. Read the spec section (the `view` tool) and split it into atomic statements: feature claims, file/symbol references, behavior rules, command names.
2. Verify each statement against code: `grep`, `symbol_index`, `list_symbols`, `project_map`; run behavior claims as checks via `bash` where cheap.
3. Classify each: MATCH (leave alone), SPEC-STALE (code is right, doc wrong), CODE-STALE (doc is right, code missing/lagged), UNDECIDED (genuine conflict — owner decides).
4. Fix SPEC-STALE rows now with the `edit` tool, tagging updated claims with evidence (SSOT P1: VERIFIED with date, or HYPOTHESIS where verification is pending).
5. List CODE-STALE rows as tasks with the `todos` tool; UNDECIDED rows go to the caller verbatim, both sides quoted.
6. Report: rows matched, doc updated, code gaps filed, conflicts escalated. Commit doc changes separately from code changes.

## Rules
- Owner directives are never edited by spec-sync — a directive that contradicts code is escalated, not rewritten ([OWNER-DIRECTIVE] tags are law).
- Preserve locked values character-for-character; a sync is not a paraphrase pass.
