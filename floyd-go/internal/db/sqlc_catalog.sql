-- SQLC-only description of SQLite's built-in catalog. This file is outside
-- migrations and is never executed against application databases.
CREATE TABLE sqlite_master (
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    tbl_name TEXT NOT NULL,
    rootpage INTEGER NOT NULL,
    sql TEXT
);
