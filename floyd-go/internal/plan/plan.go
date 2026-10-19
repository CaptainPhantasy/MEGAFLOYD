// Package plan implements per-session plan mode: one active plan per
// session, a markdown plan file, revision snapshots, and the guard and
// reminder text shared by the tools, coordinator, and UI.
package plan

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var (
	// ErrAlreadyActive is returned when entering plan mode for a session
	// that already has an active plan.
	ErrAlreadyActive = errors.New("plan mode is already active")
	// ErrNotActive is returned by operations that require an active plan.
	ErrNotActive = errors.New("plan mode is not active")
)

// State is the durable per-session plan mode state.
type State struct {
	Active bool   `json:"active"`
	ID     string `json:"id,omitempty"`
}

// Status describes the active plan and its file contents.
type Status struct {
	Active  bool
	ID      string
	Path    string
	Content string
}

// Revision is a recorded snapshot of the plan file.
type Revision struct {
	Version int
	Path    string
	SHA256  string
	Bytes   int
}

// Service stores plan state on disk so the TUI process and the agent
// process see the same active plan for a session.
type Service struct {
	dataDir string

	mu       sync.Mutex
	reminded map[string]bool
	reset    map[string]bool
}

// NewService returns a plan service rooted at dataDir. An empty dataDir
// makes every state operation fail with an explicit error.
func NewService(dataDir string) *Service {
	return &Service{
		dataDir:  dataDir,
		reminded: make(map[string]bool),
		reset:    make(map[string]bool),
	}
}

// SessionDir returns the directory holding a session's plan state.
func (s *Service) SessionDir(sessionID string) string {
	return filepath.Join(s.dataDir, "plans", safeComponent(sessionID))
}

// StatePath returns the path of the durable state file.
func (s *Service) StatePath(sessionID string) string {
	return filepath.Join(s.SessionDir(sessionID), "state.json")
}

// PlanPath returns the markdown file path for a plan.
func (s *Service) PlanPath(sessionID, planID string) string {
	return filepath.Join(s.SessionDir(sessionID), safeComponent(planID)+".md")
}

// RevisionsDir returns the directory holding a plan's revision snapshots.
func (s *Service) RevisionsDir(sessionID, planID string) string {
	return filepath.Join(s.SessionDir(sessionID), "revisions", safeComponent(planID))
}

// Enter activates plan mode for the session and returns the new plan.
func (s *Service) Enter(sessionID string) (Status, error) {
	if s.dataDir == "" {
		return Status{}, errors.New("data directory is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return Status{}, err
	}
	if state.Active {
		return Status{}, ErrAlreadyActive
	}

	id, err := newPlanID()
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(s.SessionDir(sessionID), 0o755); err != nil {
		return Status{}, fmt.Errorf("create plan directory: %w", err)
	}
	if err := s.writeState(sessionID, State{Active: true, ID: id}); err != nil {
		return Status{}, err
	}
	s.reminded[sessionID] = false
	delete(s.reset, sessionID)

	return Status{Active: true, ID: id, Path: s.PlanPath(sessionID, id)}, nil
}

// Cancel deactivates plan mode without approval.
func (s *Service) Cancel(sessionID string) error {
	return s.deactivate(sessionID)
}

// Exit deactivates plan mode after the plan is accepted.
func (s *Service) Exit(sessionID string) error {
	return s.deactivate(sessionID)
}

// Clear empties the active plan file, creating it when missing.
func (s *Service) Clear(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return err
	}
	if !state.Active || state.ID == "" {
		return nil
	}

	path := s.PlanPath(sessionID, state.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create plan directory: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("clear plan file: %w", err)
	}
	return nil
}

// Status returns the active plan. An inactive session returns a zero
// Status. A missing plan file is reported with empty content, matching
// the reference harness.
func (s *Service) Status(sessionID string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return Status{}, err
	}
	if !state.Active || state.ID == "" {
		return Status{}, nil
	}

	status := Status{
		Active: true,
		ID:     state.ID,
		Path:   s.PlanPath(sessionID, state.ID),
	}
	content, err := os.ReadFile(status.Path)
	switch {
	case err == nil:
		status.Content = string(content)
	case errors.Is(err, os.ErrNotExist):
		status.Content = ""
	default:
		return Status{}, fmt.Errorf("read plan file: %w", err)
	}
	return status, nil
}

// RecordRevision snapshots the plan file under the revisions directory
// and returns the snapshot's version, hash, and byte count. Recording is
// a no-op when no plan is active.
func (s *Service) RecordRevision(sessionID string) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return Revision{}, err
	}
	if !state.Active || state.ID == "" {
		return Revision{}, nil
	}

	content, err := os.ReadFile(s.PlanPath(sessionID, state.ID))
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		content = nil
	default:
		return Revision{}, fmt.Errorf("read plan file: %w", err)
	}

	dir := s.RevisionsDir(sessionID, state.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Revision{}, fmt.Errorf("create revisions directory: %w", err)
	}
	version, err := nextRevisionVersion(dir)
	if err != nil {
		return Revision{}, err
	}

	sum := sha256.Sum256(content)
	revision := Revision{
		Version: version,
		Path:    filepath.Join(dir, fmt.Sprintf("v%d.md", version)),
		SHA256:  hex.EncodeToString(sum[:]),
		Bytes:   len(content),
	}
	if err := os.WriteFile(revision.Path, content, 0o644); err != nil {
		return Revision{}, fmt.Errorf("write revision: %w", err)
	}

	meta, err := json.MarshalIndent(struct {
		Version int    `json:"version"`
		SHA256  string `json:"sha256"`
		Bytes   int    `json:"bytes"`
	}{revision.Version, revision.SHA256, revision.Bytes}, "", "  ")
	if err != nil {
		return Revision{}, fmt.Errorf("encode revision metadata: %w", err)
	}
	metaPath := filepath.Join(dir, fmt.Sprintf("v%d.json", version))
	if err := os.WriteFile(metaPath, meta, 0o644); err != nil {
		return Revision{}, fmt.Errorf("write revision metadata: %w", err)
	}

	return revision, nil
}

func (s *Service) deactivate(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState(sessionID)
	if err != nil {
		return err
	}
	if !state.Active {
		return nil
	}
	return s.writeState(sessionID, State{})
}

func (s *Service) readState(sessionID string) (State, error) {
	if s.dataDir == "" {
		return State{}, errors.New("data directory is not configured")
	}
	data, err := os.ReadFile(s.StatePath(sessionID))
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		return State{}, nil
	default:
		return State{}, fmt.Errorf("read plan state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse plan state: %w", err)
	}
	return state, nil
}

func (s *Service) writeState(sessionID string, state State) error {
	dir := s.SessionDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create plan directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary plan state: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write plan state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close plan state: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod plan state: %w", err)
	}
	if err := os.Rename(tmpName, s.StatePath(sessionID)); err != nil {
		return fmt.Errorf("replace plan state: %w", err)
	}
	return nil
}

func nextRevisionVersion(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("list revisions: %w", err)
	}

	maxVersion := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "v") || !strings.HasSuffix(name, ".md") {
			continue
		}
		version, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "v"), ".md"))
		if err == nil && version > maxVersion {
			maxVersion = version
		}
	}
	return maxVersion + 1, nil
}

func newPlanID() (string, error) {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate plan id: %w", err)
	}
	return "plan-" + hex.EncodeToString(raw[:]), nil
}

func safeComponent(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
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
