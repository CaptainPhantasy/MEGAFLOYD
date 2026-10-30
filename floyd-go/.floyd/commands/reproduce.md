# Reproduce — Make The Bug Happen On Demand

Turn a report into a deterministic reproduction: minimal steps, scripted, observable output. No fix happens before the failure is reproduced; if it cannot be reproduced, that is the finding.

## Usage
```
/reproduce <BUG-DESCRIPTION>
```

## Procedure
1. Extract the claim: expected behavior, actual behavior, exact error text, environment (command, version, path).
2. Reduce: strip the scenario to the smallest command sequence that still shows the failure. Prefer a one-liner over a script, a script over a session.
3. Script it: write the reproduction as a shell command or small file under the SSOT's recorded evidence folder (`docs/evidence/repro/<slug>.sh` when none is recorded) with the `write` tool — pinned env, absolute paths, deterministic flags (no parallelism, fixed seeds, timeouts set).
4. Run it via `bash` — twice. A reproduction that fires once is a flake, not a repro; note flakiness honestly if it appears.
5. Record: command, exit code, and the exact observed output (first and last lines suffice for long output). Write both to the repro file as comments.
6. If it does not reproduce: state that plainly, with the environment tried and how it differed from the report. Do not fabricate a repro to proceed.

## Rules
- The reproduction is evidence: later, /debug starts from it and /prove cites it.
- One bug, one repro file — do not let repro scripts accrete side effects.
