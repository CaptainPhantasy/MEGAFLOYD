package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rewriteJob(t *testing.T, dataDir, jobID string, mutate func(*Job)) {
	t.Helper()
	path := filepath.Join(dataDir, "jobs", jobID, jobFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var job Job
	require.NoError(t, json.Unmarshal(data, &job))
	mutate(&job)
	data, err = json.MarshalIndent(&job, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

func TestActiveSelectsNewestOpenScopedJob(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	older, err := Create(dataDir, CreateOptions{
		Objective:    "older job",
		Requirements: []string{"older requirement"},
		Scope:        []string{"internal/agent"},
	})
	require.NoError(t, err)
	newer, err := Create(dataDir, CreateOptions{
		Objective:    "newer job",
		Requirements: []string{"newer requirement"},
		Scope:        []string{"internal/jobs"},
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	rewriteJob(t, dataDir, older.ID, func(job *Job) {
		job.CreatedAt = now.Add(-2 * time.Hour)
		job.UpdatedAt = now // UpdatedAt must not influence the choice.
	})
	rewriteJob(t, dataDir, newer.ID, func(job *Job) {
		job.CreatedAt = now.Add(-time.Hour)
		job.UpdatedAt = now.Add(-time.Hour)
	})

	active, err := Active(dataDir)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, newer.ID, active.ID)
}

func TestActiveSkipsUnscopedAndCompleteJobs(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	scoped, err := Create(dataDir, CreateOptions{
		Objective:    "scoped job",
		Requirements: []string{"scoped requirement"},
		Scope:        []string{"internal/agent"},
	})
	require.NoError(t, err)
	_, err = Create(dataDir, CreateOptions{
		Objective:    "unscoped job",
		Requirements: []string{"unscoped requirement"},
	})
	require.NoError(t, err)

	active, err := Active(dataDir)
	require.NoError(t, err)
	require.NotNil(t, active)
	require.Equal(t, scoped.ID, active.ID, "an unscoped job must not hide a scoped contract")

	rewriteJob(t, dataDir, scoped.ID, func(job *Job) {
		job.Status = StatusComplete
	})
	active, err = Active(dataDir)
	require.NoError(t, err)
	require.Nil(t, active, "a completed job is never active")
}

func TestScopeAllows(t *testing.T) {
	t.Parallel()

	workingDir := "/work/project"
	scope := []string{"internal/agent", "/absolute/shared"}

	cases := []struct {
		name  string
		path  string
		allow bool
	}{
		{name: "relative inside", path: "internal/agent/agent.go", allow: true},
		{name: "scope entry itself", path: "internal/agent", allow: true},
		{name: "absolute inside", path: "/work/project/internal/agent/guard.go", allow: true},
		{name: "absolute scope entry", path: "/absolute/shared/file.go", allow: true},
		{name: "outside", path: "internal/ui/model.go", allow: false},
		{name: "prefix sibling", path: "internal/agents/new.go", allow: false},
		{name: "parent traversal", path: "../outside.go", allow: false},
		{name: "empty path", path: "", allow: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.allow, ScopeAllows(tc.path, workingDir, scope))
		})
	}
}
