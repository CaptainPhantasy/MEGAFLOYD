-- name: UpdateSessionMetadata :exec
UPDATE sessions SET metadata = ? WHERE id = ?;

-- name: SetSessionArchived :one
UPDATE sessions SET archived_at = ? WHERE id = ? RETURNING *;

-- name: MarkSessionBranch :exec
UPDATE sessions SET is_branch = 1, metadata = ? WHERE id = ?;

-- name: InsertSessionUndoRecord :exec
INSERT INTO session_undo_records (id, session_id, payload, created_at)
VALUES (?, ?, ?, strftime('%s', 'now'));

-- name: ClearSessionSummary :exec
UPDATE sessions SET summary_message_id = NULL WHERE id = ?;

-- name: CreateSessionTurn :exec
INSERT INTO session_turns (session_id, turn_id, user_message_id, completed)
VALUES (?, ?, ?, ?) ON CONFLICT (session_id, user_message_id) DO NOTHING;

-- name: GetSessionTurn :one
SELECT * FROM session_turns WHERE session_id = ? AND turn_id = ?;

-- name: GetSessionTurnByMessage :one
SELECT * FROM session_turns WHERE session_id = ? AND user_message_id = ?;

-- name: ListSessionTurns :many
SELECT * FROM session_turns WHERE session_id = ? ORDER BY turn_id ASC;

-- name: CompleteSessionTurn :exec
UPDATE session_turns SET completed = 1 WHERE session_id = ? AND user_message_id = ?;

-- name: ListTurnFiles :many
SELECT * FROM turn_file_history WHERE session_id = ? AND user_message_id = ? ORDER BY path ASC;

-- name: GetTurnFile :one
SELECT * FROM turn_file_history WHERE session_id = ? AND user_message_id = ? AND path = ?;

-- name: InsertTurnFile :exec
INSERT INTO turn_file_history (session_id, user_message_id, path, start_state, end_state)
VALUES (?, ?, ?, ?, ?);

-- name: CompleteTurnFile :exec
UPDATE turn_file_history SET end_state = ? WHERE session_id = ? AND user_message_id = ? AND path = ?;

-- name: CopySessionMessage :one
INSERT INTO messages (id, session_id, role, parts, model, provider, is_summary_message, created_at, updated_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;
