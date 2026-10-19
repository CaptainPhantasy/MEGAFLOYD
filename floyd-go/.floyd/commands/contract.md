# Contract — Declare Outcomes, Collect Evidence

Turn a task into a checkable contract before executing it: declare each requested outcome with its target and acceptance check, record an evidence receipt for every action, and finalize with per-item status. Drafted/saved/tested/committed are different states and stay different.

## Usage
```
/contract <TASK-DESCRIPTION>
```

## Procedure
1. Declare: write `.harness/contracts/<slug>.json` with the `write` tool — one item per requested outcome: id, request, target (absolute path or command), acceptance (FILE_ASSERTION on exact content/SHA-256, or TOOL_EXIT 0 on a named command).
2. Execute the work; after each mutating action, append a receipt to the contract file: action id, timestamp, command or edit performed, output excerpt, exit status. An echo is not evidence of anything but an echo.
3. Verify: run each acceptance check as a separate, later command; record its own receipt. Action and verification never share a receipt.
4. Finalize: per item set DONE (verified PASS), BLOCKED (name the external dependency, list remaining authorized actions), or FAILED (attach the error). Re-check file hashes at the end — a file changed after verification voids its PASS.
5. Report the ledger compactly: item, status, evidence ids. BLOCKED items name the exact missing input.

## Rules
- Statuses are terminal and honest: no DONE without a passing verification receipt.
- Receipts record what happened, not what was intended.
- Native Go plumbing for this loop is future work; this template runs today with bash, write, and the JSON contract file.
