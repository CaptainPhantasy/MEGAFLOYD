# Debug — Hypothesis-Driven Debugging

Diagnose a failure systematically: read the actual error, form competing hypotheses, test the cheapest first, fix the proven cause, verify with the reproduction. Starts from a reproduction (/reproduce) when one exists.

## Usage
```
/debug <FAILURE-DESCRIPTION>
```

## Procedure
1. Observe: run the reproduction (/reproduce) or the failing command; read the real error text — never debug from a paraphrase. For session-level failures, pull logs with the `floyd_logs` tool.
2. Hypothesize: write down 2-3 competing causes, each with a discriminating test that costs one command.
3. Test cheapest-first via `bash`; after each test, strike or confirm a hypothesis. Three failed strikes on the same theory means stop and report (SSOT P2), then form a new theory — never retry unchanged.
4. Locate precisely: use `grep`, `symbol_index`, and `list_symbols` to reach the exact line, not the neighborhood.
5. Fix the proven cause — smallest change that removes it (see /blast-radius before risky edits).
6. Verify: the reproduction now passes; run the affected package's tests (see /test). Report: root cause, fix, evidence.

## Rules
- No fix before a hypothesis is confirmed — "it seems related" is not confirmation.
- The final report names the mechanism (why it broke), not just the file patched.
