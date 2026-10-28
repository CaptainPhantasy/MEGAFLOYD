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
3. Use the user's configured models and reasoning settings when dispatching. Routing work does not authorize changing those settings.
4. Report the routing decision in one line: task type, route, executor. Then execute.
5. Track dispatched work: `mcp_task_status` for MCP tasks; the dispatch returns a task id for swarm submissions (see /arena).

## Rules
- Choose routine routing within the authorized task and report the choice. For a remaining material user decision, apply MegaFloyd.md section 3; do not guess or ask an already answered question.
