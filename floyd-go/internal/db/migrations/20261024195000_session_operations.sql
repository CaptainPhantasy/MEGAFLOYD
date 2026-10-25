-- +goose Up
ALTER TABLE sessions ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata) AND json_type(metadata) = 'object');
ALTER TABLE sessions ADD COLUMN archived_at INTEGER;
ALTER TABLE sessions ADD COLUMN is_branch INTEGER NOT NULL DEFAULT 0;

CREATE TABLE session_turns (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_id INTEGER NOT NULL CHECK (turn_id >= 0),
    user_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    completed INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, turn_id),
    UNIQUE (session_id, user_message_id)
);
CREATE TABLE turn_file_history (
    session_id TEXT NOT NULL,
    user_message_id TEXT NOT NULL,
    path TEXT NOT NULL,
    start_state TEXT NOT NULL,
    end_state TEXT,
    PRIMARY KEY (session_id, user_message_id, path),
    FOREIGN KEY (session_id, user_message_id) REFERENCES session_turns(session_id, user_message_id) ON DELETE CASCADE
);
CREATE TABLE session_undo_records (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    payload TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE session_undo_records;
DROP TABLE turn_file_history;
DROP TABLE session_turns;
ALTER TABLE sessions DROP COLUMN is_branch;
ALTER TABLE sessions DROP COLUMN archived_at;
ALTER TABLE sessions DROP COLUMN metadata;
