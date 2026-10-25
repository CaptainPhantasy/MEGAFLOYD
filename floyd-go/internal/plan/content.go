package plan

import (
	"fmt"
	"os"
	"path/filepath"
)

// Update writes the actual shared plan file. The previous revision is retained
// before replacing content, using the same store as plan-mode tools.
func (s *Service) Update(sessionID, content string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.recordRevisionLocked(sessionID); err != nil {
		return Status{}, err
	}
	state, err := s.readState(sessionID)
	if err != nil {
		return Status{}, err
	}
	if !state.Active {
		return Status{}, ErrNotActive
	}
	path := s.PlanPath(sessionID, state.ID)
	file, err := os.CreateTemp(filepath.Dir(path), ".plan-")
	if err != nil {
		return Status{}, err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err = file.Chmod(0644); err == nil {
		_, err = file.WriteString(content)
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Status{}, fmt.Errorf("write plan: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return Status{}, err
	}
	return Status{Active: true, ID: state.ID, Path: path, Content: content}, nil
}
