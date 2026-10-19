# Redteam — Attack The Work Adversarially

Turn the session hostile to its own output: actively try to break, abuse, or deceive the thing just built — as an attacker would. Complements /security-review (which threat-models); redteam executes attacks.

## Usage
```
/redteam <TARGET-DESCRIPTION>
```

## Procedure
1. Frame the target: what was just built, its inputs, its trust assumptions. List attack surfaces with `failure_mode_enumerator`.
2. Generate attacks per surface, prioritized:
   - Injection: feed untrusted-looking content (prompt injection in fetched pages, MCP payloads, file names) and check it does not change behavior; the paranoia layer (internal/agent, prompt-injection defenses) is the defense under test.
   - Permission bypass: craft tool calls that should be denied (writes during restricted state — try it under /freeze conditions).
   - Input abuse: empty, huge, unicode-weird, deeply nested, symlinked, and binary inputs against the changed code paths.
   - Resource exhaustion: pathological inputs that spike tokens, time, or memory.
3. Execute attacks via `bash` and tool calls against a checkpointed state (/checkpoint first — attacks may corrupt state).
4. For each attack: outcome (deflected / degraded gracefully / SUCCEEDED), evidence, and the fix if it landed.
5. Report: attacks run, hits scored, fixes needed, and whether the target is fit to ship.

## Rules
- Redteam runs against copies and checkpoints, never against production data or the owner's live state.
- A redteam that scores zero hits must show its attack list — "seems solid" is not a result.
