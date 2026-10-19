# Replay — Read The Journal And Evidence

Show what a job recorded. The `replay` tool is read-only: it never re-executes commands or repeats mutations, and its own output states that no commands are executed.

## Usage
```
/replay <JOB-ID> [REQUIREMENT]
```

## Tool
`replay` — parameters: `job_id` (required), `requirement` (optional filter).

## Procedure
1. Call `replay` with `job_id` to list the whole journal in order, then the evidence grouped by requirement.
2. Use the journal ids with the `why` tool. Use the evidence section to compare commands, exit codes, artifact hashes, output hashes, and excerpt paths.
3. To run a check again, use `bash` deliberately and record fresh evidence with `prove`. Do not describe `replay` as a re-run.

## Rules
- Journal entries show id, tool, action, detail, and result.
- Evidence rows show the requirement verdict, command, exit code, artifact hash, output hash, and excerpt path relative to the job directory.
- The journal and evidence are append-only records; changing the artifact can make old passes `unchecked`, which the tool shows as recorded.
- A `requirement` filter that matches nothing prints an empty evidence section; it does not error. Omit the filter to list everything.
