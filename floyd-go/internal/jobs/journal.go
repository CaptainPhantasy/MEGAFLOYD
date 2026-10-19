package jobs

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const journalFile = "journal.jsonl"

// JournalEntry is one recorded action in the append-only journal.
type JournalEntry struct {
	ID           string    `json:"id"`
	JobID        string    `json:"job_id"`
	Timestamp    time.Time `json:"timestamp"`
	Tool         string    `json:"tool,omitempty"`
	Action       string    `json:"action"`
	Requirement  string    `json:"requirement,omitempty"`
	ArtifactHash string    `json:"artifact_hash,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	Result       string    `json:"result,omitempty"`
}

// Journal appends an action to the job's journal. ID and Timestamp are filled
// when empty. The returned entry is the recorded form.
func Journal(dataDir, jobID string, entry JournalEntry) (*JournalEntry, error) {
	if strings.TrimSpace(entry.Action) == "" {
		return nil, errors.New("journal action is required")
	}
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, jobFile)); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return nil, fmt.Errorf("checking job record: %w", err)
	}

	if entry.ID == "" {
		entry.ID = newJournalID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	entry.JobID = jobID

	data, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encoding journal entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(filepath.Join(dir, journalFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening journal: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, fmt.Errorf("writing journal: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, fmt.Errorf("syncing journal: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("closing journal: %w", err)
	}
	return &entry, nil
}

// JournalEntries reads the journal in order. A positive limit returns the most
// recent limit entries.
func JournalEntries(dataDir, jobID string, limit int) ([]JournalEntry, error) {
	dir, err := jobDir(dataDir, jobID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, jobFile)); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return nil, fmt.Errorf("checking job record: %w", err)
	}

	f, err := os.Open(filepath.Join(dir, journalFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening journal: %w", err)
	}
	defer f.Close()

	var entries []JournalEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading journal: %w", err)
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

// Why returns the journal entry with the exact action id. It never infers an
// answer: an absent id is ErrUnknownAction.
func Why(dataDir, jobID, actionID string) (*JournalEntry, error) {
	if strings.TrimSpace(actionID) == "" {
		return nil, fmt.Errorf("%w: empty action id", ErrUnknownAction)
	}
	entries, err := JournalEntries(dataDir, jobID, 0)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == actionID {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrUnknownAction, actionID)
}

func newJournalID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("j-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("j-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(buf[:]))
}
