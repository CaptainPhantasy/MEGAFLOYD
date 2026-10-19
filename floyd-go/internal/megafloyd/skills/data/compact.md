# Compact — Shrink Session Context

Free context window space by archiving the full record and replacing it with a tiered summary. The session keeps working without losing the thread.

## Usage
```
/compact
```

## Procedure
1. Measure: call the `context_status` tool to get current token usage and window headroom.
2. Archive: call `transcript_export` so the full conversation is preserved on disk before anything is dropped.
3. Offload: move reference material (long outputs, file dumps, research notes) into the scratchpad with `manage_scratchpad`.
4. Summarize in three tiers, following the agent summarizer rules (internal/agent/summarizer.go):
   - Recent: last exchange kept verbatim.
   - Middle: one-line summaries per topic.
   - Pinned: decisions, locked values, open tasks — kept verbatim.
5. If the chassis exposes a built-in compact command in the command palette (ctrl+p), prefer it and skip manual summarization.
6. Verify: call `context_status` again and report before/after usage.

## Rules
- Never drop: locked values, owner directives, failing test names, open questions.
- Report the compaction result in two lines: tokens freed, what was archived where.
