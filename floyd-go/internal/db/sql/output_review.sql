-- name: CountOutputReviewActivations :one
SELECT COUNT(*) FROM output_review_activation WHERE id = 1;

-- name: SeedLegacyOutputReviewPublic :exec
INSERT INTO output_review_public (message_id, session_id, revision, source_hash, parts, state, receipt)
SELECT id, session_id, 0, '', parts, 'legacy_visible', '{"reviewer":"published-before-activation"}'
FROM messages
WHERE role = 'assistant'
  AND NOT EXISTS (SELECT 1 FROM output_review_private p WHERE p.message_id = messages.id)
ON CONFLICT(message_id) DO NOTHING;

-- name: CreateOutputReviewActivation :exec
INSERT INTO output_review_activation (id) VALUES (1);

-- name: GetLatestOutputReviewPrivate :one
SELECT * FROM output_review_private
WHERE message_id = ? ORDER BY revision DESC LIMIT 1;

-- name: CountOutputReviewCorrections :one
SELECT COUNT(*) FROM output_review_corrections WHERE message_id = ? AND revision = ?;

-- name: CountOutputReviewSource :one
SELECT COUNT(*) FROM output_review_private WHERE message_id = ? AND source_hash = ?;

-- name: CreateOutputReviewPrivate :exec
INSERT INTO output_review_private (message_id, session_id, revision, kind, source_hash, parts)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetOutputReviewPublic :one
SELECT * FROM output_review_public WHERE message_id = ?;

-- name: UpsertOutputReviewPublic :exec
INSERT INTO output_review_public (message_id, session_id, revision, source_hash, parts, state, receipt)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(message_id) DO UPDATE SET
  revision = excluded.revision,
  source_hash = excluded.source_hash,
  parts = excluded.parts,
  state = excluded.state,
  receipt = excluded.receipt;

-- name: GetOutputReviewCorrection :one
SELECT receipt FROM output_review_corrections WHERE message_id = ? AND revision = ?;

-- name: GetLatestOutputReviewCorrection :one
SELECT receipt FROM output_review_corrections WHERE message_id = ? ORDER BY revision DESC LIMIT 1;

-- name: CreateOutputReviewCorrection :exec
INSERT INTO output_review_corrections (message_id, revision, receipt) VALUES (?, ?, ?);

-- name: GetLatestOutputReviewEvent :one
SELECT * FROM output_review_events WHERE message_id = ? ORDER BY cursor DESC LIMIT 1;

-- name: ListOutputReviewEvents :many
SELECT * FROM output_review_events WHERE session_id = ? AND cursor > ? ORDER BY cursor;

-- name: CreateOutputReviewEvent :exec
INSERT INTO output_review_events (message_id, session_id, revision, parts, state) VALUES (?, ?, ?, ?, ?);

-- name: CreateOutputReviewAttempt :exec
INSERT INTO output_review_attempts (message_id, source_hash, candidate, issue) VALUES (?, ?, ?, ?);
