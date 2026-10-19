# Route — Send Work To The Right Executor

Classify the incoming task and route it: handle inline, dispatch a sub-agent, or parallelize through the swarm. Routing is how the session stays cheap on simple work and strong on hard work.

## Usage
```
/route <TASK-DESCRIPTION>
```

## Procedure
1. Classify the task with the `complexity_estimator` tool: leaf change, multi-file feature, research, or mechanical sweep.
2. Pick the route:
   - Trivial/mechanical (edit a file, run a check): do it inline now.
   - Deep/slow (repo-wide research, risky refactor): dispatch to the `task` sub-agent (coordinator's named agents — internal/agent/coordinator.go) so heavy tool output stays out of the main window.
   - Independent parallel work (N unrelated checks): `parallel_bash` in one call.
   - Competitive/evaluative (compare approaches): /arena via the swarm coordinator.
3. For model fit, adjust the dial before dispatch: /effort low for mechanical work, high for architecture (see /effort).
4. Report the routing decision in one line: task type, route, executor. Then execute.
5. Track dispatched work: `mcp_task_status` for MCP tasks; the dispatch returns a task id for swarm submissions (see /arena).

## Rules
- Routing is a decision, not a question — never ask the caller which route; classify and act, and say what you chose.
