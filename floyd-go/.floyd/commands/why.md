# Why — Explain The Reasoning Behind A Decision

Answer "why is it this way?" with evidence: trace the decision to the conversation, commit, or doc that made it, and separate recorded rationale from inference. The anti-hallucination rule applies fully to history.

## Usage
```
/why <CODE-OR-DECISION-REFERENCE>
```

## Procedure
1. Locate the artifact: file:line, config key, or decision name (`grep`, `symbol_index`).
2. Trace its history: `git log --follow -p -- <path>` via `bash` to the introducing commit; read the commit message and the diff's intent.
3. Trace the conversation: `transcript_export` the session(s) that touched it, and search the export for the decision moment; `query_floyd_archive` reaches prior sessions.
4. Check the docs: the SSOT and plan files for a recorded decision ([OWNER-DIRECTIVE] outranks everything).
5. Answer in three parts, labeled:
   - RECORDED: quoted rationale from commit/doc/transcript, with source cited.
   - INFERRED: what the code structure implies, clearly marked as inference.
   - UNKNOWN: what no source answers.
6. If the recorded reason no longer holds, say so and name what would decide a change (owner question, test, benchmark).

## Rules
- Never present inference as recorded rationale — the labels are the point of this skill.
- "It seems they wanted" is banned; quote or mark UNKNOWN.
