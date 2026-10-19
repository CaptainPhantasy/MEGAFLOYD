Record a verification proof for one requirement, or show the proofs already recorded. Every receipt carries the command, exit code, output SHA-256, excerpt path, artifact SHA-256, and timestamp.

<usage>
- `action=record` with `job_id`, `requirement`, `command`, `exit_code`, and `output`. Pass `artifact` or `artifact_hash` when the contract has no declared artifact.
- `action=show` with `job_id` lists evidence per requirement, including invalidation reasons.
</usage>

<features>
- Exit code 0 means PASSED; any other exit code means FAILED.
- The artifact hash is captured at recording time. If the artifact changes later, the proof no longer counts toward completion.
- Output is stored as an excerpt file and hashed, so the receipt can be checked without trusting memory.
</features>

<tips>
- Quote the decisive output lines, not the whole log.
- A failed check is evidence too: record it with its nonzero exit code.
- Re-record after any artifact change; stale proofs are reported as UNCHECKED, not passed.
</tips>
