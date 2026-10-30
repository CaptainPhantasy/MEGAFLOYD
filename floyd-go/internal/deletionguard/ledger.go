package deletionguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RecordSchema identifies the durable session record layout. A record with any
// other schema is refused so a future format is never read as approval.
const RecordSchema = "deletion-confirmation/v1"

// humanSource marks the only accepted origin of a confirmation. Model text
// cannot write an entry with this source.
const humanSource = "human"

var (
	// ErrSessionRecordUnavailable reports a session record that exists but
	// cannot be interpreted.
	ErrSessionRecordUnavailable = errors.New("deletion session record is unavailable")
	// ErrNotConfirmed reports a permanent deletion without both confirmations.
	ErrNotConfirmed = errors.New("permanent deletion needs two confirmations for its exact target set")
)

// Confirmation is one human confirmation for an exact target set.
type Confirmation struct {
	ID string `json:"id"`
	// Stage is 1 for "identify the exact items" and 2 for "confirm the
	// consequences". Both stages are required and must be separate answers.
	Stage int `json:"stage"`
	// TargetSet is the digest of the exact items this answer covers.
	TargetSet string   `json:"target_set"`
	Targets   []string `json:"targets"`
	// Source is always "human".
	Source     string `json:"source"`
	Question   string `json:"question,omitempty"`
	RecordedAt string `json:"recorded_at"`
}

// Execution records that one approved deletion already ran.
type Execution struct {
	ID          string   `json:"id"`
	TargetSet   string   `json:"target_set"`
	ApprovalSet string   `json:"approval_set,omitempty"`
	Targets     []string `json:"targets"`
	ToolCallID  string   `json:"tool_call_id,omitempty"`
	RecordedAt  string   `json:"recorded_at"`
}

// Record is the durable per-session deletion record.
type Record struct {
	Schema        string         `json:"schema"`
	SessionID     string         `json:"session_id"`
	Confirmations []Confirmation `json:"confirmations"`
	Executions    []Execution    `json:"executions"`
	Approvals     []Approval     `json:"approvals,omitempty"`
	Attempts      []Attempt      `json:"attempts,omitempty"`
}

// ConfirmationsFor returns the first human confirmation for each stage of one
// target set.
func (r Record) ConfirmationsFor(targetSet string) map[int]Confirmation {
	out := make(map[int]Confirmation, 2)
	for _, confirmation := range r.Confirmations {
		if confirmation.TargetSet != targetSet || confirmation.Source != humanSource {
			continue
		}
		if confirmation.Stage != 1 && confirmation.Stage != 2 {
			continue
		}
		if _, ok := out[confirmation.Stage]; ok {
			continue
		}
		out[confirmation.Stage] = confirmation
	}
	return out
}

// Confirmed reports whether both stages for one target set are recorded as
// separate human answers, in order.
func (r Record) Confirmed(targetSet string) bool {
	stages := r.ConfirmationsFor(targetSet)
	first, hasFirst := stages[1]
	second, hasSecond := stages[2]
	if !hasFirst || !hasSecond || first.ID == second.ID {
		return false
	}
	firstAt, err := time.Parse(time.RFC3339Nano, first.RecordedAt)
	if err != nil {
		return false
	}
	secondAt, err := time.Parse(time.RFC3339Nano, second.RecordedAt)
	if err != nil {
		return false
	}
	return !secondAt.Before(firstAt)
}

// Executed reports whether an approved deletion already ran for one target
// set, which means the same removal must not be repeated.
func (r Record) Executed(targetSet string) (Execution, bool) {
	for _, execution := range r.Executions {
		if execution.TargetSet == targetSet {
			return execution, true
		}
	}
	return Execution{}, false
}

// Ledger stores the per-session deletion record on disk so confirmations
// survive a restart and stay separate from conversation text.
type Ledger struct {
	dataDir string
	mu      *sync.Mutex
}

// NewLedger returns a ledger rooted at dataDir.
var ledgerLocks sync.Map

func NewLedger(dataDir string) *Ledger {
	key, err := filepath.Abs(dataDir)
	if err != nil {
		key = dataDir
	}
	lock, _ := ledgerLocks.LoadOrStore(key, &sync.Mutex{})
	return &Ledger{dataDir: dataDir, mu: lock.(*sync.Mutex)}
}

// Dir returns the directory holding a session's deletion record.
func (l *Ledger) Dir(sessionID string) string {
	return filepath.Join(l.dataDir, "deletions", safeComponent(sessionID))
}

// Path returns a session's deletion record path.
func (l *Ledger) Path(sessionID string) string {
	return filepath.Join(l.Dir(sessionID), "record.json")
}

// Read loads a session record. A missing file is an empty record; a malformed,
// foreign or unreadable file is an error so permanent deletion fails closed.
func (l *Ledger) Read(sessionID string) (Record, error) {
	if strings.TrimSpace(l.dataDir) == "" {
		return Record{}, fmt.Errorf("%w: the data directory is not configured", ErrSessionRecordUnavailable)
	}
	if strings.TrimSpace(sessionID) == "" {
		return Record{}, fmt.Errorf("%w: the session ID is empty", ErrSessionRecordUnavailable)
	}
	data, err := os.ReadFile(l.Path(sessionID))
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		return Record{Schema: RecordSchema, SessionID: sessionID}, nil
	default:
		return Record{}, fmt.Errorf("%w: %v", ErrSessionRecordUnavailable, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrSessionRecordUnavailable, err)
	}
	if record.Schema != RecordSchema {
		return Record{}, fmt.Errorf("%w: unexpected schema %q", ErrSessionRecordUnavailable, record.Schema)
	}
	if record.SessionID != sessionID {
		return Record{}, fmt.Errorf("%w: the record belongs to session %q", ErrSessionRecordUnavailable, record.SessionID)
	}
	return record, nil
}

// AppendConfirmation records one human confirmation. Only a human answer may
// create an entry, so model text cannot satisfy the gate.
func (l *Ledger) AppendConfirmation(sessionID string, confirmation Confirmation) error {
	if confirmation.Source != humanSource {
		return fmt.Errorf("a deletion confirmation must come from a human answer, not %q", confirmation.Source)
	}
	if confirmation.Stage != 1 && confirmation.Stage != 2 {
		return fmt.Errorf("a deletion confirmation needs stage 1 or 2, not %d", confirmation.Stage)
	}
	if confirmation.TargetSet == "" || len(confirmation.Targets) == 0 {
		return errors.New("a deletion confirmation needs an exact target set")
	}
	if confirmation.ID == "" {
		confirmation.ID = uuid.NewString()
	}
	if confirmation.RecordedAt == "" {
		confirmation.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	record, err := l.Read(sessionID)
	if err != nil {
		return err
	}
	record.Confirmations = append(record.Confirmations, confirmation)
	return l.write(sessionID, record)
}

// AppendExecution records that an approved deletion ran.
func (l *Ledger) AppendExecution(sessionID string, execution Execution) error {
	if execution.TargetSet == "" || len(execution.Targets) == 0 {
		return errors.New("an execution record needs an exact target set")
	}
	if execution.ID == "" {
		execution.ID = uuid.NewString()
	}
	if execution.RecordedAt == "" {
		execution.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	record, err := l.Read(sessionID)
	if err != nil {
		return err
	}
	record.Executions = append(record.Executions, execution)
	return l.write(sessionID, record)
}

func (l *Ledger) write(sessionID string, record Record) error {
	dir := l.Dir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create deletion record directory: %w", err)
	}
	record.Schema = RecordSchema
	record.SessionID = sessionID
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deletion record: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".record-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary deletion record: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write deletion record: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync deletion record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close deletion record: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod deletion record: %w", err)
	}
	if err := os.Rename(tmpName, l.Path(sessionID)); err != nil {
		return fmt.Errorf("replace deletion record: %w", err)
	}
	return nil
}

func safeComponent(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	result := b.String()
	if result == "" || result == "." || result == ".." {
		return "session"
	}
	return result
}
