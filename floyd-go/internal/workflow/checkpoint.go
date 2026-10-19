package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Checkpoint represents a saved workflow state for recovery
type Checkpoint struct {
	ID           string            `json:"id"`
	WorkflowName string            `json:"workflow_name"`
	WorkflowID   string            `json:"workflow_id"`
	Status       WorkflowStatus    `json:"status"`
	Steps        []Step            `json:"steps"`
	CurrentStep  int               `json:"current_step"`
	Context      map[string]string `json:"context,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Error        string            `json:"error,omitempty"`
}

// CheckpointService manages workflow checkpoint persistence
type CheckpointService interface {
	Save(checkpoint *Checkpoint) error
	Load(id string) (*Checkpoint, error)
	List(workflowName string) ([]*Checkpoint, error)
	Delete(id string) error
	GetLatest(workflowName string) (*Checkpoint, error)
}

type checkpointService struct {
	baseDir string
}

// NewCheckpointService creates a new checkpoint service
func NewCheckpointService(workingDir string) CheckpointService {
	return &checkpointService{
		baseDir: filepath.Join(workingDir, ".floyd", "workflows"),
	}
}

func (s *checkpointService) checkpointPath(id string) string {
	return filepath.Join(s.baseDir, id+".json")
}

func (s *checkpointService) ensureDir() error {
	return os.MkdirAll(s.baseDir, 0755)
}

// Save persists a checkpoint to disk
func (s *checkpointService) Save(checkpoint *Checkpoint) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	checkpoint.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	path := s.checkpointPath(checkpoint.ID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	return nil
}

// Load retrieves a checkpoint from disk
func (s *checkpointService) Load(id string) (*Checkpoint, error) {
	path := s.checkpointPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// List returns all checkpoints, optionally filtered by workflow name
func (s *checkpointService) List(workflowName string) ([]*Checkpoint, error) {
	if err := s.ensureDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	var checkpoints []*Checkpoint
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")
		checkpoint, err := s.Load(id)
		if err != nil {
			continue
		}

		if workflowName == "" || checkpoint.WorkflowName == workflowName {
			checkpoints = append(checkpoints, checkpoint)
		}
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].UpdatedAt.After(checkpoints[j].UpdatedAt)
	})

	return checkpoints, nil
}

// Delete removes a checkpoint from disk
func (s *checkpointService) Delete(id string) error {
	path := s.checkpointPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}
	return nil
}

// GetLatest returns the most recent checkpoint for a workflow
func (s *checkpointService) GetLatest(workflowName string) (*Checkpoint, error) {
	checkpoints, err := s.List(workflowName)
	if err != nil {
		return nil, err
	}

	if len(checkpoints) == 0 {
		return nil, nil
	}

	return checkpoints[0], nil
}

// NewCheckpoint creates a new checkpoint with a generated ID
func NewCheckpoint(workflowName, workflowID string, steps []Step) *Checkpoint {
	now := time.Now()
	return &Checkpoint{
		ID:           uuid.New().String(),
		WorkflowName: workflowName,
		WorkflowID:   workflowID,
		Status:       StatusPending,
		Steps:        steps,
		CurrentStep:  0,
		Context:      make(map[string]string),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
