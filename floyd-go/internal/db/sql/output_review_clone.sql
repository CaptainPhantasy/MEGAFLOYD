-- name: CloneOutputReviewPrivate :exec
INSERT INTO output_review_private(message_id, session_id, revision, kind, source_hash, parts)
SELECT sqlc.arg(new_message_id), sqlc.arg(destination_session_id), revision, kind, source_hash, parts
FROM output_review_private WHERE output_review_private.message_id = sqlc.arg(source_message_id);

-- name: CloneOutputReviewPublic :exec
INSERT INTO output_review_public(message_id, session_id, revision, source_hash, parts, state, receipt)
SELECT sqlc.arg(new_message_id), sqlc.arg(destination_session_id), revision, source_hash, parts, state, receipt
FROM output_review_public WHERE output_review_public.message_id = sqlc.arg(source_message_id);

-- name: CloneOutputReviewCorrections :exec
INSERT INTO output_review_corrections(message_id, revision, receipt)
SELECT sqlc.arg(new_message_id), revision, receipt
FROM output_review_corrections WHERE output_review_corrections.message_id = sqlc.arg(source_message_id);

-- name: CloneOutputReviewAttempts :exec
INSERT INTO output_review_attempts(message_id, source_hash, candidate, issue)
SELECT sqlc.arg(new_message_id), source_hash, candidate, issue
FROM output_review_attempts WHERE output_review_attempts.message_id = sqlc.arg(source_message_id);

-- name: ListOutputReviewCloneEvents :many
SELECT message_id, revision, parts, state FROM output_review_events
WHERE session_id = ? ORDER BY cursor;

-- name: CreateOutputReviewCloneEvent :exec
INSERT INTO output_review_events(message_id, session_id, revision, parts, state)
VALUES (?, ?, ?, ?, ?);
