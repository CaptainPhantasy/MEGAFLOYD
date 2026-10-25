# MegaFloyd hooks

Hooks run trusted user-configured shell commands at native lifecycle boundaries. They use the embedded shell, inherit the process environment, and receive the actual event payload on stdin. They have the same trust as other executable configuration; they are not a sandbox. No hooks or behavioral policy are enabled by this upgrade.

For read-only inspection, see [hook_explain, hook_trace, and the hooks CLI](inspection.md). Inspection reports effective matching/order and stored execution metadata without invoking hook commands. Offline inspection reads declarative JSON; it does not execute floydrc or command expansion.

## Events and authority

| Event | Boundary | Authority |
| --- | --- | --- |
| `PreToolUse` | Before a tool, including native subagent tools and direct shell dispatch | Synchronous deny/halt, explicit per-call approval (existing behavior), and validated input rewrite; explicitly `required` checks can fail the action |
| `PostToolUse` | After a successful tool result | Observational; an explicitly `required` failed check makes the result incomplete, with the tool's effects already applied |
| `PostToolUseFailure` | Actual tool error or completed nonzero shell exit | Observational |
| `ActionRejected` | An action rejected before execution | Observational |
| `ToolCancelled` | Actual tool cancellation | Observational |
| `PermissionRequest` | Native permission evaluation begins | Observational; cannot answer or grant permission |
| `PermissionResolved` | Native permission evaluation resolves, including automatic resolution | Observational |
| `SessionStart` | A session begins its first run | Synchronous additional context only |
| `Stop` | A run reaches its terminal state | Observational; its decisions cannot change the terminal run outcome |

Aliases are case-insensitive and accept snake_case. `ToolFailure` maps to `PostToolUseFailure`, `ToolCanceled` to `ToolCancelled`, and `RunComplete` to `Stop`. Unknown legacy optional event keys remain accepted but are not dispatched. Modern controls on unknown events are rejected.

A synchronous `PreToolUse` hook's explicit `allow` retains the existing per-call preapproval behavior. Success without an explicit `allow` leaves normal native permissions in place. Only `PreToolUse` applies approval, deny/halt, or input rewrites. Permission hooks and all other observers have no approval authority. A post-check cannot undo a write. Inspection tools bypass hook dispatch themselves so inspection cannot execute configured commands; ordinary tool availability filters still apply.

## Configuration and order

```json
{
  "hooks": {
    "PreToolUse": [
      {"name": "check-input", "matcher": "^bash$", "command": "./scripts/check-tool-input", "required": true, "timeout": 10}
    ],
    "PostToolUse": [
      {"name": "observe-result", "command": "./scripts/record-tool-result", "async": true}
    ]
  }
}
```

Only configure scripts that exist and that you trust. A required check is opt-in and is not a built-in policy. Shell configuration also supports `hook add EVENT --command CMD --name NAME --required`, `--sequential`, and `--async`.

- Existing optional synchronous hooks run in parallel and aggregate in configuration order.
- If any matching synchronous hook is `required` or `sequential`, the matching synchronous batch runs in configuration order. A pre-tool rewrite becomes the next handler's input. Denial, cancellation, or a required failure stops dependent handlers.
- `required` is available only for synchronous `PreToolUse` and `PostToolUse`. A nonzero exit, timeout, cancellation, malformed structured result, or explicit denial fails the required check. Exit zero with no output succeeds.
- `sequential` changes ordering, not error authority. Optional ordinary command errors remain advisory.
- `async` is available only for observational events other than `SessionStart`. It cannot combine with `required` or `sequential`. Results cannot alter the action, authorize permissions, revise input, or start a model turn.
- Canonical event keys are merged first, then aliases in lexical order. Each list retains its order. Duplicate matching commands with the same timeout and authority flags run once; different authority or timeout stays separate. Preview uses the exact same planner.

Timeout is in seconds, default 30. Cancellation reaches the shell interpreter and its external processes. An uncooperative interpreter can be abandoned after a one-second grace period; this does not prove that an arbitrary external side effect was rolled back. Background observers are tracked, cancelled with their owning runtime, and drained during application shutdown before the audit database closes.

## Versioned input and output

The [v1 JSON Schema](../../internal/hooks/payload.schema.json) defines stdin. Native fields include `version`, `event_id`, RFC3339 `timestamp`, event/session/cwd, tool name and parsed `tool_input`; relevant boundaries also provide tool/run/parent IDs, observed outcome, duration, exit code, response and interruption state. Tool input cannot replace these native fields. All handlers in one event batch share its event ID. `ValidatePayload` validates the envelope; the tool's own schema remains responsible for `tool_input`.

The environment includes `MEGAFLOYD_EVENT`, `MEGAFLOYD_TOOL_NAME`, `MEGAFLOYD_SESSION_ID`, `MEGAFLOYD_CWD`, `MEGAFLOYD_PROJECT_DIR`, and input command/file-path variables where present. Existing `FLOYD_*` and `CRUSH_*` aliases remain available.

```json
{"version":1,"decision":"allow","context":"Additional context","updated_input":{"command":"safe rewritten command"}}
```

Decision is `none`, `allow`, or `deny`. `halt:true` requests a turn halt at the pre-tool boundary. `context` accepts a string or string array. `updated_input` is a shallow object patch with literal top-level keys; unchanged input keys are retained. The existing `hookSpecificOutput` compatibility envelope is also supported.

Exit `2` blocks a pre-tool action; exit `49` requests a halt. Other nonzero exits remain advisory for optional hooks. Optional malformed output remains ignored for compatibility. Required structured output rejects duplicate fields, unsupported versions, invalid types/decisions and invalid object patches. Command stderr may supply an intentional denial reason to the caller; do not put secrets there.

## Records and privacy

Each invocation has a stable execution ID and running/terminal records, with event/session/tool/run/parent correlation, configuration index, command SHA-256, safe display name, mode, timing, decision, exit status and error category. The trace store does **not** persist raw commands, input, response, stderr, or denial reasons. Unnamed hooks display as `hook[index]`. Logs likewise omit raw commands and stderr. Configured commands still receive full input/environment by design; metadata privacy is not a claim that arbitrary hook scripts redact their own output.

## Existing rewrite example

The retained [RTK rewrite example](examples/rtk-rewrite.sh) demonstrates the
existing pre-tool shallow-patch contract. Installing any example remains an
explicit configuration choice; this upgrade does not configure it automatically.

## Design references

The lifecycle and structured input design was checked against the official [Claude Code hook reference](https://code.claude.com/docs/en/hooks) and [Gemini CLI hook reference](https://geminicli.com/docs/hooks/reference/) on 2026-10-25. MegaFloyd's authority and ordering are the rules above; compatibility does not imply support for every event or output field of another product.
