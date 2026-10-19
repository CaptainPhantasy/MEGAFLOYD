# Bisect — Find The Commit That Broke It

Drive git bisect to pinpoint the commit that introduced a defect: pick good/bad refs, script the test, let the binary search run, then examine the culprit.

## Usage
```
/bisect <BAD-REF> <GOOD-REF> <TEST-COMMAND>
```

## Procedure
1. Confirm the failure reproduces at `<BAD-REF>` and passes at `<GOOD-REF>` — run the test command at both via `bash`. If either assumption is wrong, stop and report; bisect cannot start from lies.
2. Start the search: `git bisect start <bad> <good>` via `bash`.
3. At each step bisect checks out, run the test command, then `git bisect good` or `git bisect bad` accordingly. The test must be deterministic — flaky tests poison bisect; stabilize first (/reproduce).
4. When bisect names the first bad commit: `git bisect reset`, then inspect the culprit — `git show <culprit>` and `get_active_diff` context — to confirm the mechanism, not just the identity.
5. Report: culprit commit + subject, the failing mechanism it introduced, and the smallest fix proposal.

## Rules
- Build/test steps can be slow: run them as background jobs (/shadow) and poll with `job_output` so the session stays responsive.
- Never leave a bisect in progress: `git bisect reset` always, even on interruption.
