# Drift — Compare Changes To Declared Scope

Compare the files a job changed against the paths its contract declared. The `drift` tool reports work that escaped the plan; it never edits files.

## Usage
```
/drift <JOB-ID>
```

## Tool
`drift` — parameters: `job_id` (required), `paths` (optional).

## Procedure
1. Call `drift` with `job_id`. By default it reads changed paths from `git status --porcelain --untracked-files=all` in the working directory.
2. Pass `paths` only to review an explicit list; this is required outside a git repository.
3. Read the report: declared scope, changed-path count, and each violation with its reason.
4. For every violation, either revert the change or accept it by declaring a new contract with the corrected scope. `drift` does not modify the contract.

## Rules
- Scope matching is prefix-based: `internal/jobs` covers `internal/jobs/jobs.go`.
- An empty scope declares nothing in scope, so every changed path is a violation.
- Violation reasons are exact: `outside declared scope` or `no scope declared in contract`.
- A violation is a finding, not an automatic failure. Report it before claiming completion.
