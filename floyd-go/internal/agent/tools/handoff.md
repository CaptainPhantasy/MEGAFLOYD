Write a portable handoff packet for a job so a fresh session can continue without rediscovering state.

<usage>
- Provide `job_id`.
- The packet is written to `jobs/<job_id>/handoff.md` and returned in full.
</usage>

<features>
- Objective, status, artifact path and current SHA-256.
- Per-requirement verdicts, invalidation reasons, and latest evidence.
- Exact next action, recent journal entries, and file locations.
- The packet is self-contained: evidence excerpt paths are relative to the job directory.
</features>

<tips>
- Write the handoff before a long pause or a context reset.
- Evidence over adjectives: the packet quotes exit codes and hashes, not summaries.
- A successor should read the packet before any tool call.
</tips>
