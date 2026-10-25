# Explain and trace hooks

`hook_explain` previews the current workspace's effective hook configuration.
`hook_trace` reads that workspace's recorded event and job history. Both are
native MegaFloyd tools, available to the same agent in the terminal, browser,
and desktop. They do not execute configured hooks, even when a hook matcher
would otherwise match every tool. Normal per-session tool availability still
applies.

## Preview without execution

From a terminal connected to an existing MegaFloyd workspace:

```sh
megafloyd --cwd /path/to/project hooks explain PreToolUse --tool bash
megafloyd --cwd /path/to/project hooks explain PostToolUse --tool edit --json
```

The command uses read-only requests to the already-running native server. Use
`--host` when that server uses an explicit host. It does not start or attach a
workspace, load executable configuration, or start a model. If no matching
workspace is active, it reports that fact.

To inspect a JSON configuration snapshot offline:

```sh
megafloyd hooks explain PreToolUse --tool bash --config ./floyd.json --json
```

This reads and validates JSON only. It does not execute `floydrc`, expand
`$(...)`, merge other configuration files, or run hook commands. Consequently,
it describes that snapshot rather than claiming it is the active configuration.

The preview uses the dispatch engine's own matching and deduplication rules. Each row contains the effective configuration index, command hash,
match result, duplicate reference, execution mode, order, and capabilities.
Raw commands are omitted. The effective key is not a claim about which source
file supplied a merged setting.

For parallel hooks, order describes result composition; it does not promise
start or completion order. Ordered hooks execute in order. Async observers
finish later. An observational hook cannot undo an action or grant permission.
Synchronous PreToolUse hooks retain their existing ability to explicitly allow
the current native permission request; `can_approve` reports this capability.
Required post-action checks can report failure but cannot undo the completed
action. Capability fields describe what a hook may affect, not what its command
will actually return. Use a trace for actual outcomes.

## Trace recorded outcomes

```sh
megafloyd --cwd /path/to/project hooks trace --session SESSION_ID --json
megafloyd --cwd /path/to/project hooks trace --tool-call TOOL_CALL_ID --json
megafloyd --data-dir /path/to/existing/native-data hooks trace --job JOB_ID --json
```

The default CLI data directory comes from the matching active workspace. The
explicit `--data-dir` form can read a closed workspace's existing store. Both
forms open history read-only, without migrations or creating a database.
The agent tool defaults to its current session; its reader is bound to the
application's workspace store and accepts no filesystem path.

Optional session, tool-call, event, and job filters combine. Records are ordered
by sequence. The JSON page includes `next` and `has_more`; repeat the same
filters with `--after NEXT` to continue. `--limit` defaults to 100 and accepts
1 through 500. These are page sizes, not a truncation of stored history.

Records identify native outcomes and correlation, hook lifecycle state, hook
decisions, mode,
command hashes, exit status, and timing. Hook inputs, stdout, stderr, raw shell
commands, and free-text reasons are not stored in this inspection history.
An empty page means no matching records were stored, which can include a store
created before hook tracing existed. It does not prove that no earlier action
ran. Access errors and corrupt storage are reported as errors.

These inspection tools do not activate any proposed behavioral policy, run an
output reviewer, retry a job, or change configuration or provider settings.
