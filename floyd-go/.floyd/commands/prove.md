# Prove — Record Evidence For A Requirement

Record the real check behind a contract requirement. Every receipt stores the command, exit code, output SHA-256, an output excerpt, the artifact SHA-256, and a timestamp. The tool name is `prove`.

## Usage
```
/prove <JOB-ID> <REQUIREMENT-ID>
```

## Tool
`prove` — actions: `record`, `show`.

## Procedure
1. Run the actual check with `bash` and capture the exit status plus the decisive output lines.
2. Call `prove` with `action=record`, `job_id`, `requirement` (the generated id, e.g. `tests-pass`), `command`, `exit_code`, and `output`.
   - Exit code `0` records PASSED; any other exit code records FAILED.
   - The artifact hash defaults to the contract's artifact. Pass `artifact` or `artifact_hash` only when the evidence must bind to a different file or a pre-computed hash; `artifact_version` adds an optional label.
3. Call `prove` with `action=show`, `job_id`, and optionally `requirement`, to read the verdicts back.
4. Re-record after any artifact change. A receipt bound to an older hash no longer counts.

## Rules
- The latest receipt decides: a later failed receipt overrides an earlier pass.
- `unchecked` means no evidence, or evidence recorded against a different artifact hash. It never means passed.
- Output is written to an excerpt file and hashed, so quote the lines that decide the result.
- A failed check is evidence too: record it with its nonzero exit code instead of hiding it.
- Do not claim a result in prose that `prove show` does not report.
