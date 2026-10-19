Reconstruct interrupted work from durable job records. Recovery reads state; it never guesses it and never re-runs a mutation to "make sure".

<usage>
- Omit `job_id` to summarize every unfinished contract with its last three journal entries.
- Provide `job_id` to inspect one contract in detail.
</usage>

<features>
- Shows each requirement's verdict and the exact next action.
- Names the last journal entries with their ids so follow-up questions can use `why`.
- Reports jobs whose evidence is complete but whose completion was never marked.
</features>

<tips>
- After recovery, inspect the working tree before continuing; a half-mutated file must be completed or reverted, never left ambiguous.
- If the same failure repeats three times unchanged, stop and report instead of retrying a fourth time.
</tips>
