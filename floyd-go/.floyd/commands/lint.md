# Lint — Format And Lint

Bring the code to the project's format and lint standard: format first, then lint, fix what's fixable, report the remainder with reasons.

## Usage
```
/lint [path]
```

## Procedure
1. Format: in this repo `task fmt` (gofumpt -w .); scope to a path when given. Fallbacks in order: goimports, gofmt.
2. Lint: `task lint` (golangci-lint), plus `task lint:log` for the log-capitalization rule. Scoped: `golangci-lint run <path>`.
3. Auto-fix what the tooling offers (`task lint:fix`); re-run format after fixes.
4. Re-run lint until clean, or until remaining findings are judgment calls. Two unchanged failing retries = stop and report (SSOT P2).
5. Report: commands run, findings fixed, findings remaining with file:line and the rule name. Offer /debug for findings whose cause is unclear.

## Rules
- Lint clean is a merge gate in this repo (SSOT P4) — do not report work done while lint is red.
- Never disable a linter rule to clear a finding; fix the code or argue the rule with the owner.
