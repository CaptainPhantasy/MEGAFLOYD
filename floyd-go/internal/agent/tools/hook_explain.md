Preview the current workspace's configured hooks for an event and exact tool name.
This inspection does not run hook commands, request permission, or change configuration.

Each entry identifies its effective configuration index and command hash. Matching,
deduplication, execution mode, and ordering come from the same engine that dispatches
hooks. A zero order means the hook will not run; inspect the match, duplicate, and
error fields to find why. Parallel entries have a result-composition order, not a
promised start or completion order. Ordered entries execute in their stated order.
Async observers report later and cannot grant permission or change the action.

A synchronous PreToolUse hook can explicitly allow the current native permission
request. Observers cannot. Required post-action checks can report failure but cannot
undo the action that already happened.

Capability fields describe what the hook is permitted to affect. They do not predict
its command's result. The preview reports effective merged config keys, not the
originating file or a claimed execution. Commands and secret-bearing inputs are
omitted. Use hook_trace to inspect actual persisted outcomes.
