# Context — Context Status Report

Show what is in the context window right now: token usage, window headroom, files and skills loaded, and what is close to eviction. Reporting only — to reclaim space use /compact.

## Usage
```
/context
```

## Procedure
1. Call the `context_status` tool: report tokens used, context window size, remaining headroom.
2. Call the `floyd_info` tool: report loaded config, active skills, LSP servers, and MCP servers.
3. List the heaviest items in context (largest tool results, oldest unused content) and flag anything that should move to the scratchpad via `manage_scratchpad`.
4. If usage is above 80% of the window, say so and suggest /compact or /budget.

## Rules
- Report only. No mutations.
- Keep the report to one screen: numbers first, then the top offenders.
