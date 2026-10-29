// Package hookstore retains metadata about hook execution in the workspace DB.
// It never stores hook commands, tool input, output, or free-form error text.
package hookstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/legacy-ai/floyd/internal/db"
)

const MaxLimit = 500

type Query struct {
	SessionID  string `json:"session_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	EventID    string `json:"event_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	After      int64  `json:"after,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// Record contains only native correlation and execution metadata.
type Record struct {
	Seq              int64  `json:"seq"`
	ID               string `json:"id"`
	EventID          string `json:"event_id"`
	JobID            string `json:"job_id,omitempty"`
	Event            string `json:"event"`
	SessionID        string `json:"session_id"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	HookName         string `json:"hook_name,omitempty"`
	State            string `json:"state"`
	Outcome          string `json:"outcome,omitempty"`
	Decision         string `json:"decision,omitempty"`
	StartedAt        int64  `json:"started_at"`
	FinishedAt       int64  `json:"finished_at,omitempty"`
	DurationMS       int64  `json:"duration_ms,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	HookCount        int    `json:"hook_count,omitempty"`
	Halt             bool   `json:"halt,omitempty"`
	InputRewrite     bool   `json:"input_rewrite,omitempty"`
	ConfigIndex      int    `json:"config_index"`
	CommandHash      string `json:"command_hash,omitempty"`
	Mode             string `json:"mode,omitempty"`
	ErrorKind        string `json:"error_kind,omitempty"`
	Required         bool   `json:"required,omitempty"`
	Rewritten        bool   `json:"rewritten,omitempty"`
}

type Page struct {
	Records []Record `json:"records"`
	Next    int64    `json:"next"`
	HasMore bool     `json:"has_more"`
}

type Reader interface {
	List(context.Context, Query) (Page, error)
}

type Store struct {
	db         *sql.DB
	errorMu    sync.Mutex
	writeError error
}

func New(conn *sql.DB) *Store { return &Store{db: conn} }

func (s *Store) Append(ctx context.Context, record Record) error {
	if s.db == nil {
		return errors.New("hook execution store is read-only or unavailable")
	}
	if record.ID == "" || record.EventID == "" || record.Event == "" {
		return errors.New("hook record requires native event and execution IDs")
	}
	body, err := json.Marshal(record)
	if err == nil {
		err = db.New(s.db).CreateHookExecutionRecord(ctx, db.CreateHookExecutionRecordParams{ID: record.ID, SessionID: record.SessionID, EventID: record.EventID, ToolCallID: record.ToolCallID, JobID: record.JobID, Body: string(body)})
	}
	if err != nil {
		s.errorMu.Lock()
		if s.writeError == nil {
			s.writeError = fmt.Errorf("hook execution record could not be retained: %w", err)
		}
		s.errorMu.Unlock()
	}
	return err
}

func (s *Store) List(ctx context.Context, query Query) (page Page, resultErr error) {
	defer func() {
		s.errorMu.Lock()
		resultErr = errors.Join(resultErr, s.writeError)
		s.errorMu.Unlock()
	}()
	page = Page{Records: []Record{}, Next: query.After}
	if query.After < 0 || query.Limit < 0 || query.Limit > MaxLimit {
		return page, fmt.Errorf("hook trace limit must be 0..%d and cursor must be nonnegative", MaxLimit)
	}
	limit := query.Limit
	if limit == 0 {
		limit = 100
	}
	if s.db == nil {
		return page, nil
	}
	queries := db.New(s.db)
	exists, err := queries.HookExecutionRecordsExist(ctx)
	if err != nil {
		return page, err
	}
	if exists == 0 {
		return page, nil
	}
	rows, err := queries.ListHookExecutionRecords(ctx, db.ListHookExecutionRecordsParams{
		AfterSeq: query.After, SessionID: query.SessionID, ToolCallID: query.ToolCallID,
		EventID: query.EventID, JobID: query.JobID, ResultLimit: int64(limit + 1),
	})
	if err != nil {
		return page, err
	}
	for _, row := range rows {
		if len(page.Records) == limit {
			page.HasMore = true
			break
		}
		var record Record
		if err := json.Unmarshal([]byte(row.Body), &record); err != nil {
			return page, fmt.Errorf("invalid stored hook record %d: %w", row.Seq, err)
		}
		record.Seq = row.Seq
		page.Records = append(page.Records, record)
		page.Next = row.Seq
	}
	return page, nil
}

type emptyCloser struct{}

func (emptyCloser) Close() error { return nil }

// OpenReader uses the existing account/workspace DB without migrations or
// application initialization. Missing history is empty; read failures are not.
func OpenReader(dataDir string) (Reader, io.Closer, error) {
	if dataDir == "" {
		return nil, nil, errors.New("hook trace data directory is empty")
	}
	path := filepath.Join(dataDir, "floyd.db")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return New(nil), emptyCloser{}, nil
	} else if err != nil {
		return nil, nil, err
	}
	conn, err := db.ConnectReadOnly(context.Background(), path)
	if err != nil {
		return nil, nil, err
	}
	return New(conn), conn, nil
}
