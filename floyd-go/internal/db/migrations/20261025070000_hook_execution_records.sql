-- +goose Up
CREATE TABLE hook_execution_records (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    tool_call_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    body TEXT NOT NULL
);
CREATE INDEX hook_execution_session_seq ON hook_execution_records(session_id, seq);
CREATE INDEX hook_execution_event_seq ON hook_execution_records(event_id, seq);
CREATE INDEX hook_execution_tool_seq ON hook_execution_records(tool_call_id, seq);
CREATE INDEX hook_execution_job_seq ON hook_execution_records(job_id, seq);

-- +goose Down
DROP TABLE hook_execution_records;
