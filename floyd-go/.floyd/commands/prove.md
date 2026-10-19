# Prove — Verification Proof For A Claim

Back a specific claim ("it works", "tests pass", "the fix landed") with captured evidence: named checks, real output, hashes, timestamps. What /contract is to a whole task, /prove is to one claim.

## Usage
```
/prove <CLAIM>
```

## Procedure
1. Restate the claim as a checkable statement: name the artifact (file, command, behavior) and what "true" means for it.
2. Choose checks that directly test the claim — build for "compiles", tests for "works", the reproduction (/reproduce) for "fixed", /invariants for "safe". Cite the artifact: exact path, and its SHA-256 (`shasum -a 256 <file>` via `bash`) so the proof is bound to bytes.
3. Run the checks via `bash`; capture command, exit code, and the decisive output lines verbatim. Long suites run under /shadow.
4. Write the proof record to `.harness/proofs/<slug>.md` with the `write` tool: claim, checks (commands), results (exit codes + output), artifact hashes, timestamp.
5. Verdict, one of: PROVEN (every check passed, hashes match), REFUTED (a check failed — show it), UNPROVEN (a needed check could not run — name it). Partial passes are UNPROVEN, with the gap named.

## Rules
- The verdict follows the evidence, never the reverse; a claim you cannot check is UNPROVEN, not PROVEN-with-caveats.
- A proof older than the artifact's last change is void — re-run on change.
