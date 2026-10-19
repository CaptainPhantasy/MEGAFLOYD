# 10 — PR Summary + Test Plan

```md
Create a high-signal PR summary and executable test plan.

## Inputs
- Branch diff from main
- Commits in PR

## Output format
1) Why this PR exists (1-3 bullets)
2) What changed (scoped, not verbose)
3) Risk/impact areas
4) Test plan checklist (manual + automated)
5) Rollback notes

## Rule
Summary must reflect all commits since branch divergence.
```