Read recorded hook events, jobs, and outcomes from this workspace's own history store.
The current session is the default scope. An explicit session_id selects another
session only within this same workspace. Optional tool_call_id, event_id, and job_id
filters combine, so you can follow one action and its asynchronous observations.

Records preserve sequence, cause/correlation, execution mode, state, decisions,
timestamps, and command hashes. Raw commands, tool input, stdout, stderr, and free-text
reasons are not recorded in this inspection history. An empty page means no matching
records were stored; it is not proof that no action or hook ever ran. Storage errors
are returned explicitly.

Use next as after when has_more is true. Reading never executes or retries a hook,
changes permissions, starts an agent, or traverses another account's storage.
