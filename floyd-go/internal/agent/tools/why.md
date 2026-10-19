Answer "why did this happen?" from the job journal. The journal records actions with stable ids; this tool quotes the matching entry and never invents a rationale.

<usage>
- Provide `job_id` and the exact `action_id` shown in another job tool's output.
- The reply labels the answer RECORDED when the id exists and UNKNOWN when it does not.
</usage>

<features>
- Returns timestamp, tool, action, requirement, detail, artifact hash, and result from the entry itself.
- An unknown id produces UNKNOWN with no inference.
- Reads only; it does not modify the journal.
</features>

<tips>
- Use ids returned by `contract`, `prove`, `handoff`, and `recover` output.
- If the id is unknown, list the journal through `replay` and use a real id.
</tips>
