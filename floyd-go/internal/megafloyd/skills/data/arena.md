# Arena — Run Competing Agents, Judge A Winner

Dispatch the same task to two or more agents running in parallel and judge their outputs against stated criteria. The swarm coordinator (internal/agent/swarm_coordinator.go) is the machinery; the winner feeds /promote.

## Aliases
`/race` — same skill, same job. Arena and race are synonyms here (SSOT §3E combining rule).

## Usage
```
/arena <N> <TASK-DESCRIPTION> [CRITERIA]
```

## Procedure
1. Isolate the lanes: one git worktree per contender (`git worktree add ../arena-<i>`) so each result is a diff against the same base, not a mess in one tree.
2. Dispatch: submit the same prompt once per contender through the swarm coordinator — `Submit(prompt, agentName)` returns a task id per lane; use the coordinator's named agents (e.g. `coder`, `task`). State the acceptance criteria inside each prompt so contenders self-check.
3. Await: `Await(taskID)` per lane (poll `mcp_task_status` for MCP-routed work). Contenders that exceed the stated time box lose by default.
4. Collect diffs: in each worktree, `git diff` (or `get_active_diff`) plus the contender's final report.
5. Judge against the criteria stated in step 2, in order: correctness evidence (tests run and passing), blast radius (/blast-radius per contender), code quality, cost (tokens/time). Rank and name the winner with the reason in one sentence.
6. Clean the arena: remove losing worktrees (`git worktree remove`), keep the winner's for /promote. Report the ranking table.

## Rules
- Criteria are declared before the race starts — judging after seeing outputs is rigging.
- A tie is a tie: say it and let the caller break it.
