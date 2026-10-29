-- name: CreateHookExecutionRecord :exec
INSERT INTO hook_execution_records(id, session_id, event_id, tool_call_id, job_id, body)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListHookExecutionRecords :many
SELECT seq, body FROM hook_execution_records
WHERE seq > sqlc.arg(after_seq)
  AND (sqlc.arg(session_id) = '' OR session_id = sqlc.arg(session_id))
  AND (sqlc.arg(tool_call_id) = '' OR tool_call_id = sqlc.arg(tool_call_id))
  AND (sqlc.arg(event_id) = '' OR event_id = sqlc.arg(event_id))
  AND (sqlc.arg(job_id) = '' OR job_id = sqlc.arg(job_id))
ORDER BY seq LIMIT sqlc.arg(result_limit);

-- name: HookExecutionRecordsExist :one
SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'hook_execution_records';
