Show the recorded journal and evidence for a job. This is a read-only replay view: it never re-executes commands or repeats mutations.

<usage>
- Provide `job_id`.
- Optionally provide `requirement` to narrow the evidence section.
</usage>

<features>
- Lists journal entries in order with ids, tools, actions, details, and results.
- Lists each requirement's evidence with command, exit code, artifact hash, output hash, and excerpt path.
- States explicitly that nothing was re-executed.
</features>

<tips>
- Use this to find a real action id for `why`.
- To re-run checks, use `bash` deliberately and record fresh evidence with `prove`; replay itself stays passive.
- Compare artifact hashes across receipts to spot proofs recorded before a change.
</tips>
