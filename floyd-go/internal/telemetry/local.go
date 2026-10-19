package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// LocalStore provides a local SQLite-based event store for telemetry.
// It enables offline analytics and the `floyd stats` dashboard.
type LocalStore struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

// LocalEvent represents a persisted telemetry event in the local store.
type LocalEvent struct {
	ID         int64           `json:"id"`
	Type       EventType       `json:"type"`
	Timestamp  time.Time       `json:"timestamp"`
	Properties json.RawMessage `json:"properties"`
}

// StatsSummary holds aggregated statistics from the local store.
type StatsSummary struct {
	TotalEvents  int64               `json:"total_events"`
	EventsByType map[EventType]int64 `json:"events_by_type"`
	FirstEvent   time.Time           `json:"first_event"`
	LastEvent    time.Time           `json:"last_event"`
}

// NewLocalStore opens or creates a local SQLite telemetry database
// at the given path. If path is empty, it defaults to
// ".floyd/telemetry.db" relative to the working directory.
func NewLocalStore(path string) (*LocalStore, error) {
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		path = filepath.Join(wd, ".floyd", "telemetry.db")
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create telemetry directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open local telemetry db: %w", err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init telemetry schema: %w", err)
	}

	store := &LocalStore{db: db, path: path}

	slog.Debug("Local telemetry store initialized", "path", path)
	return store, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		type       TEXT    NOT NULL,
		timestamp  TEXT    NOT NULL DEFAULT (datetime('now')),
		properties TEXT    NOT NULL DEFAULT '{}'
	);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	`
	_, err := db.Exec(schema)
	return err
}

// Record stores a telemetry event in the local database.
func (s *LocalStore) Record(eventType EventType, properties map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	props, err := json.Marshal(properties)
	if err != nil {
		return fmt.Errorf("marshal event properties: %w", err)
	}

	_, err = s.db.Exec(
		"INSERT INTO events (type, timestamp, properties) VALUES (?, ?, ?)",
		string(eventType),
		time.Now().UTC().Format(time.RFC3339),
		string(props),
	)
	if err != nil {
		return fmt.Errorf("insert local event: %w", err)
	}

	return nil
}

// Query retrieves events matching the given criteria.
func (s *LocalStore) Query(ctx context.Context, eventType EventType, limit int) ([]LocalEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "SELECT id, type, timestamp, properties FROM events"
	args := []any{}

	if eventType != "" {
		query += " WHERE type = ?"
		args = append(args, string(eventType))
	}

	query += " ORDER BY id DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query local events: %w", err)
	}
	defer rows.Close()

	var events []LocalEvent
	for rows.Next() {
		var e LocalEvent
		var ts string
		if err := rows.Scan(&e.ID, &e.Type, &ts, &e.Properties); err != nil {
			return nil, fmt.Errorf("scan local event: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		events = append(events, e)
	}

	return events, nil
}

// GetStats returns aggregated statistics from the local store.
func (s *LocalStore) GetStats(ctx context.Context) (*StatsSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := &StatsSummary{
		EventsByType: make(map[EventType]int64),
	}

	// Total events
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&stats.TotalEvents)
	if err != nil {
		return nil, err
	}

	// Events by type
	rows, err := s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM events GROUP BY type")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t EventType
		var count int64
		if err := rows.Scan(&t, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.EventsByType[t] = count
	}
	rows.Close()

	// First and last event
	s.db.QueryRowContext(ctx, "SELECT MIN(timestamp), MAX(timestamp) FROM events").Scan(
		&stats.FirstEvent, &stats.LastEvent,
	)

	return stats, nil
}

// Close closes the underlying database connection.
func (s *LocalStore) Close() error {
	return s.db.Close()
}
