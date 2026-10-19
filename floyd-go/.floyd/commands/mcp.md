# Mcp — Manage MCP Servers And Tools

Inspect and exercise MCP connections: which servers are live, what tools and resources they expose, and what queued MCP tasks are doing.

## Usage
```
/mcp [server-name]
```

## Procedure
1. Inventory: call the `floyd_info` tool — it reports configured MCP servers and their connection state.
2. For a named server: list what it exposes with the `list_mcp_resources` tool; read a resource with `read_mcp_resource`.
3. Check async work: call `mcp_task_status` for outstanding MCP task results.
4. Configuration changes (add/remove servers) live in `floydrc` via the `mcp` builtin or `floyd.json` — see docs/config/. After editing config, reload and re-check with `floyd_info`.
5. Report: per server — connected or not, tool count, resource count, queued tasks.

## Rules
- MCP servers can carry secrets in their config (`$(...)` expansion) — never print resolved credentials in the report.
- A server listed in config but not connected is a finding, not a pass: name it and its connection error.
