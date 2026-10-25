-- +goose Up
CREATE TABLE IF NOT EXISTS output_review_activation (id INTEGER PRIMARY KEY CHECK(id=1), activated_at INTEGER NOT NULL DEFAULT (unixepoch()));

 CREATE TABLE IF NOT EXISTS output_review_private (
   message_id TEXT NOT NULL, session_id TEXT NOT NULL, revision INTEGER NOT NULL,
   kind TEXT NOT NULL, source_hash TEXT NOT NULL, parts TEXT NOT NULL,
   PRIMARY KEY(message_id,revision));
 CREATE TABLE IF NOT EXISTS output_review_public (
   message_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, revision INTEGER NOT NULL,
   source_hash TEXT NOT NULL, parts TEXT NOT NULL, state TEXT NOT NULL, receipt TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS output_review_attempts (
   id INTEGER PRIMARY KEY AUTOINCREMENT, message_id TEXT NOT NULL,
   source_hash TEXT NOT NULL, candidate TEXT NOT NULL, issue TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS output_review_corrections (
   message_id TEXT NOT NULL, revision INTEGER NOT NULL, receipt TEXT NOT NULL,
   PRIMARY KEY(message_id,revision));
 CREATE TABLE IF NOT EXISTS output_review_events (
   cursor INTEGER PRIMARY KEY AUTOINCREMENT, message_id TEXT NOT NULL,
   session_id TEXT NOT NULL, revision INTEGER NOT NULL, parts TEXT NOT NULL,
   state TEXT NOT NULL);

-- +goose Down
DROP TABLE output_review_activation;
DROP TABLE output_review_events;
DROP TABLE output_review_corrections;
DROP TABLE output_review_attempts;
DROP TABLE output_review_public;
DROP TABLE output_review_private;
