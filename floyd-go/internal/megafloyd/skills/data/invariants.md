# Invariants — Enforce Statements That Must Stay True

Capture the project's invariants (conditions that must always hold), encode each one as a check that a machine can run, and run them all. An invariant without a check is an opinion.

## Usage
```
/invariants [check|add <INVARIANT-STATEMENT>]
```

## Procedure
1. Load the invariant list from `.harness/invariants.md` (the `view` tool); if absent, derive candidates from MegaFloyd.md and the user-approved project requirements recorded in the SSOT. Keep proposed checks distinct from approved requirements; this list records checks, not a second behavioral policy.
2. `add`: append the statement to `.harness/invariants.md` with a check command beside it (the `edit` tool). Every entry is: statement, check command, last-verified date.
3. `check` (default): run every check command via `bash` (independent checks batched in one `parallel_bash` call).
4. For value-shape invariants (paths, JSON, URLs, non-empty fields) prefer the shared validators in internal/validation when wiring Go-side checks.
5. Report a table: invariant, PASS/FAIL, evidence (command + key output line). Any FAIL names the exact violation and the file:line.

## Rules
- Invariants are cheap and fast — a check that takes minutes belongs in CI, not here.
- Running invariants after a change is mandatory before claiming the change safe (see /prove).
