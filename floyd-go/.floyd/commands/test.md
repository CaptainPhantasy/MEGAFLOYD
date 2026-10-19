# Test — Run The Project's Tests

Run the right tests for the change: affected package first, then the full suite. Report real pass/fail counts from actual output — a test suite nobody ran is a rumor.

## Usage
```
/test [package-or-path]
```

## Procedure
1. Detect the runner from the repo (go-task, Makefile, package.json). In this repo: `task test` (equals `go test -race -failfast ./...`); a single package is `go test ./internal/<pkg>`; a single test adds `-run TestName`.
2. With an argument, run the affected package first: `go test -race ./<path>` via `bash`.
3. Then run the full suite for anything touching shared code: `task test`.
4. Read the output: report pass count, fail count, and for each failure the test name plus the first error line.
5. Long suites run as background jobs (/shadow); poll with `job_output` — never block silently, never abandon.
6. Report: commands run, exit codes, counts, and the failures verbatim-enough to act on. Offer /debug for each failure.

## Rules
- Green claims require shown output from this run — last week's pass does not count.
- Never weaken, skip, or delete a test to achieve green (SSOT owner directive: never weaken checks).
