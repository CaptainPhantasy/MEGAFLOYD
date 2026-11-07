package workflow

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/megafloyd"
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
	pathErr error
}

// NewCheckpointService creates a new checkpoint service
func NewCheckpointService(workingDir string, selected ...string) CheckpointService {
	var explicit string
	if len(selected) > 0 {
		explicit = selected[0]
	}
	dataDir, err := megafloyd.PersonalDataDir(workingDir, explicit)
	return &checkpointService{baseDir: filepath.Join(dataDir, "workflows"), pathErr: err}
}

func (s *checkpointService) checkpointPath(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) || strings.ContainsRune(id, 0) {
		return "", fmt.Errorf("invalid checkpoint ID")
	}
	return filepath.Join(s.baseDir, id+".json"), nil
}

func (s *checkpointService) ensureDir() error {
	if s.pathErr != nil {
		return s.pathErr
	}
	return os.MkdirAll(s.baseDir, 0o700)
}

// Save persists a checkpoint to disk
func (s *checkpointService) Save(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is required")
	}
	path, err := s.checkpointPath(checkpoint.ID)
	if err != nil {
		return err
	}
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	checkpoint.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	temp, err := os.CreateTemp(s.baseDir, ".checkpoint-*")
	if err != nil {
		return fmt.Errorf("create checkpoint temporary file: %w", err)
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("replace checkpoint file: %w", err)
	}

	return nil
}

// Load retrieves a checkpoint from disk
func (s *checkpointService) Load(id string) (*Checkpoint, error) {
	if s.pathErr != nil {
		return nil, s.pathErr
	}
	path, err := s.checkpointPath(id)
	if err != nil {
		return nil, err
	}
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
	if s.pathErr != nil {
		return s.pathErr
	}
	path, err := s.checkpointPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}
	return nil
}

// cloneCheckpoint isolates a worker's mutable state from callers and events.
func cloneCheckpoint(checkpoint *Checkpoint) *Checkpoint {
	if checkpoint == nil {
		return nil
	}
	copy := *checkpoint
	copy.Context = maps.Clone(checkpoint.Context)
	copy.Steps = append([]Step(nil), checkpoint.Steps...)
	cloneTime := func(value *time.Time) *time.Time {
		if value == nil {
			return nil
		}
		copy := *value
		return &copy
	}
	copy.CompletedAt = cloneTime(checkpoint.CompletedAt)
	for i := range copy.Steps {
		step := &copy.Steps[i]
		step.Requires = append([]string(nil), step.Requires...)
		step.Config = cloneConfig(step.Config)
		step.StartedAt = cloneTime(step.StartedAt)
		step.CompletedAt = cloneTime(step.CompletedAt)
	}
	return &copy
}

func cloneConfig(config map[string]interface{}) map[string]interface{} {
	copy := maps.Clone(config)
	for key, value := range copy {
		copy[key] = cloneConfigValue(value)
	}
	return copy
}

func cloneConfigValue(value any) any {
	switch value := value.(type) {
	case map[string]interface{}:
		return cloneConfig(value)
	case []interface{}:
		copy := make([]interface{}, len(value))
		for i, child := range value {
			copy[i] = cloneConfigValue(child)
		}
		return copy
	default:
		return value
	}
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
