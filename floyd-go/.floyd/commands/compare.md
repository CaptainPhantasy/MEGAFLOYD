# Compare — Side-By-Side Evaluation

Put two existing things (files, outputs, approaches, sessions) next to each other and produce a decision-grade comparison: differences, trade-offs, recommendation. /arena produces contenders; /compare evaluates them — or any pair the caller names.

## Usage
```
/compare <A> <B> [CRITERIA]
```

## Procedure
1. Load both sides: files via `view`; diffs via `get_active_diff` or `git diff`; session outputs via `transcript_export`. Normalize to the same shape before comparing (line up sections, strip noise).
2. Diff mechanically first: `git diff --no-index <a> <b>` via `bash` for files; enumerate the actual differences before interpreting them.
3. When the pair is two code approaches, score each with `complexity_estimator` and check dependents with `symbol_index` + `grep` — blast radius (/blast-radius) is a criterion.
4. Weigh against the caller's criteria, or the default set: correctness, maintainability, risk, cost.
5. Report: a differences table (aspect | A | B), the trade-off in one sentence per aspect, and a recommendation with its deciding reason. Mark the recommendation as advice — the caller decides.

## Rules
- Compare evidence, not reputations: "B is the newer library" is not a comparison row.
- Differences found but not evaluated are listed as such — never silently dropped.
