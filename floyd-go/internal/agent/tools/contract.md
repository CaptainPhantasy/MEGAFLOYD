Create and manage a durable job contract. A contract declares the objective, one or more checkable requirements, optional scope paths, and an optional artifact whose SHA-256 binds every proof.

<usage>
- `action=create` with `objective` and `requirements` (and optionally `scope`, `artifact`). Returns the job id.
- `action=list` shows every contract and its passed/total count.
- `action=show` with `job_id` shows per-requirement status and evidence.
- `action=complete` with `job_id` marks the job complete. Completion is refused unless every requirement has passing evidence recorded for the artifact's current SHA-256.
</usage>

<features>
- Requirements are independent and each needs its own passing evidence.
- Artifact hashes make stale proofs visible: changing the artifact invalidates earlier passing evidence.
- Declared scope gives the `drift` tool a reference for change review.
- Records persist under the data directory at `jobs/<job_id>/`.
</features>

<tips>
- Declare the artifact at create time so `prove` can bind evidence automatically.
- Keep requirements short and checkable, such as "go test ./internal/jobs exits 0".
- Mark complete only after `prove` shows every requirement PASSED.
</tips>
