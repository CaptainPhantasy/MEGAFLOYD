# Recover — Resume Broken Or Interrupted Work

Pick up work that stopped badly — crashed session, failed run, abandoned loop — by reconstructing state from durable records, then continuing or stopping per the three-strike rule. Recovery reads state; it never guesses it.

## Usage
```
/recover [session-id-or-worktree]
```

## Procedure
1. Find the wreckage: `floyd session list` and `floyd session show <id>` for the last session state; `/ralph-status` if a ralph loop was running; the `floyd_logs` tool for how it ended; run markers (internal/agent/run_marker.go) for interrupted runs.
2. Reconstruct: `transcript_export` the session — read the last exchanges to establish exactly where work stopped: what was in flight, what was verified, what was claimed-but-not-checked.
3. Check the ground truth: `get_active_diff` and `git status` — files may be half-mutated. A half-mutated target is either completed, reverted to a checkpoint (/restore), or re-declared; it is never left mid-state.
4. Rebuild the plan: reload open todos (`todos` tool); write the continuation point (one sentence: current state, next action).
5. Continue work. If the same failure recurs: three strikes and stop-and-report (SSOT P2) — same failure, three unchanged retries = report, not a fourth try.
6. Report: what was recovered, what was lost, the next action taken.

## Rules
- Never re-run a mutation "to make sure" — inspect the target's current state first; duplicated side effects are worse than the crash.
- Recovery from a ralph loop resumes the loop (/loop) rather than restarting the task from zero.
