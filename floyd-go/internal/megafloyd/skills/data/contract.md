# Contract — Declare Checkable Outcomes

Turn a task into a durable job contract before doing the work. The `contract` tool records the objective, one or more checkable requirements, the paths the job may change, and an optional artifact whose SHA-256 binds every proof.

## Usage
```
/contract <TASK-DESCRIPTION>
```

## Tool
`contract` — actions: `create`, `list`, `show`, `complete`.
Records live under `<data-directory>/jobs/<job_id>/`; the default data directory is `.floyd` in the working directory.

## Procedure
1. Restate the task as one objective sentence plus checkable requirements, e.g. "go test ./internal/jobs exits 0". Each requirement becomes a generated id such as `tests-pass`.
2. Call `contract` with `action=create`, `objective`, `requirements`, and, when applicable, `artifact` and `scope`.
   - `artifact` is a file path; relative paths resolve against the working directory, and its current SHA-256 is captured at create time.
   - `scope` lists the paths this job may change. An empty scope makes every changed path drift.
3. Execute the work. Record every check with the `prove` tool; never hand-write receipts into job files.
4. Call `contract` with `action=complete` only after `prove` shows every requirement passed. Completion is refused unless each requirement has passing evidence for the artifact's current SHA-256.
5. Report the job id, per-requirement verdicts, and any blocking reasons. A refusal is a result to act on, not an error to bypass.

## Rules
- `show` and `list` are read-only; `show` reports each requirement as `passed`, `failed`, or `unchecked`.
- Changing the artifact after recording evidence turns earlier passes `unchecked`; re-run the check and record new evidence.
- `complete` never overrides missing evidence, and does not mark requirements individually.
