# Recover — Reconstruct Unfinished Job State

Rebuild interrupted work from durable job records. The `recover` tool reads state; it never guesses it and never re-runs a mutation.

## Usage
```
/recover [JOB-ID]
```

## Tool
`recover` — parameter: `job_id` (optional).

## Procedure
1. Omit `job_id` to summarize every unfinished contract with its last three journal entries. Provide `job_id` to inspect one contract.
2. Read each requirement's verdict and reason, the reported next action, and the journal ids.
3. Check ground truth with `get_active_diff` or `git status`. A half-mutated file must be completed or reverted; it is never left mid-state.
4. Continue at the reported next action: run the check, record it with `prove`, then call `contract` with `action=complete`.
5. If no job exists but work was interrupted, reconstruct the state from git and declare a contract before continuing.

## Rules
- Never re-run a mutation to confirm it happened; inspect the target first.
- Duplicated side effects are worse than lost time.
- The same failure three times unchanged means stop and report, not a fourth retry (SSOT §5.4 P2).
- Recovery reconstructs contracts, not sessions or transcripts.
