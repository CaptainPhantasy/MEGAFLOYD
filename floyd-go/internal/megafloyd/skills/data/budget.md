# Budget — Token Budget Guardrails

Watch the session's token spend against its budget, warn before the ceiling, and take corrective action before the window fills. The enforcement machinery is the TokenBudget tracker (internal/agent/budget.go); this skill drives it with live numbers.

## Usage
```
/budget [set <LIMIT-TOKENS> | status]
```

## Procedure
1. `status` (default): call the `context_status` tool — report tokens used this session, context window size, headroom, and percent of budget consumed.
2. If a limit was given, record it and compare live usage against it on every subsequent `context_status` reading.
3. When usage crosses the warn threshold (TokenBudget's WarnPercent; default 80%):
   - Report the overrun in one line, then act — do not keep spending silently.
   - Reclaim space: /compact (archive + tiered summary).
   - Offload detail: move reference material to the scratchpad with `manage_scratchpad`.
   - Delegate heavy reads: dispatch a sub-agent (task agent) so the big tool results land outside the main window; check on it with `mcp_task_status` if it is an MCP task.
4. At the hard limit: stop generating, state the limit plainly, and ask whether to compact or continue over budget.

## Rules
- Budget numbers come from `context_status` — never estimate token usage by eye.
- A budget report is numbers first: used / limit / percent, then the action taken.
