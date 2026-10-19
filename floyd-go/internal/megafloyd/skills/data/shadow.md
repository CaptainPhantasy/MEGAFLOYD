# Shadow — Run Work In The Background

Launch a long-running task as a background shell job so the main conversation stays free. Monitor without blocking; surface results when ready; kill cleanly when abandoned.

## Usage
```
/shadow <COMMAND-OR-TASK-DESCRIPTION>
```

## Procedure
1. Launch the work as a background job with the `bash` tool (its background-job mode; chassis job machinery lives in internal/shell). Note the job id it returns.
2. Do not block. Continue the main task.
3. Poll progress with the `job_output` tool using the job id. Poll on natural breaks, not in a tight loop.
4. When the job finishes: report exit status and the tail of its output; triage failures where they landed.
5. To abandon a job, use the `job_kill` tool with the job id — never leave orphaned jobs running.
6. For independent long checks, launch several via `parallel_bash` instead and collect all results at once.

## Rules
- Always report the job id when launching so it can be found later.
- A background job is not done until `job_output` shows completion; never claim its result from the launch acknowledgment alone.
