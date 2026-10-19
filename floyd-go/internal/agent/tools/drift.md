Compare the current changes against the paths a contract declared in scope. Drift reports work that escaped the plan; it never edits files.

<usage>
- Provide `job_id`. By default changed paths come from `git status --porcelain --untracked-files=all` in the working directory.
- Provide `paths` to review an explicit list instead of asking git.
</usage>

<features>
- A scope entry matches itself and everything below it, so `internal/jobs` covers `internal/jobs/jobs.go`.
- An empty scope means nothing can be shown in scope, and every change is reported.
- Violations name the path and the reason: outside declared scope, or no scope declared.
</features>

<tips>
- Run this before reporting completion when the contract declared scope.
- A violation is a finding, not an automatic failure: either revert the change, or update the contract scope deliberately.
</tips>
