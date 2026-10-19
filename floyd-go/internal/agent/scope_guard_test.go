package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/stretchr/testify/require"
)

func createScopeTestJob(t *testing.T, dataDir string, scope []string) *jobs.Job {
	t.Helper()
	job, err := jobs.Create(dataDir, jobs.CreateOptions{
		Objective:    "scope guard test",
		Requirements: []string{"write stays inside scope"},
		Scope:        scope,
	})
	require.NoError(t, err)
	return job
}

func markScopeTestJobComplete(t *testing.T, dataDir string, job *jobs.Job) {
	t.Helper()
	path := filepath.Join(dataDir, "jobs", job.ID, "job.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var stored jobs.Job
	require.NoError(t, json.Unmarshal(data, &stored))
	stored.Status = jobs.StatusComplete
	data, err = json.MarshalIndent(&stored, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

func runScopeGuardedTool(
	t *testing.T,
	ctx context.Context,
	guardDataDir string,
	plans *plan.Service,
	workingDir string,
	toolName string,
	input map[string]string,
) (fantasy.ToolResponse, bool) {
	t.Helper()
	ran := false
	inner := planGuardRecordingTool{name: toolName, ran: &ran}
	wrapped := wrapToolsWithScopeGuard(
		[]fantasy.AgentTool{inner}, guardDataDir, plans, workingDir,
	)[0]
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	resp, err := wrapped.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  toolName,
		Input: string(raw),
	})
	require.NoError(t, err)
	return resp, ran
}

func TestScopeGuardMatrix(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	dataDir := t.TempDir()
	job := createScopeTestJob(t, dataDir, []string{"internal/agent"})
	ctx := planTestContext("session-1")

	insideAbsolute := filepath.Join(workingDir, "internal/agent/scope_guard.go")
	cases := []struct {
		name       string
		toolName   string
		input      map[string]string
		wantDenied bool
	}{
		{
			name:     "write inside scope is allowed",
			toolName: tools.WriteToolName,
			input:    map[string]string{"file_path": "internal/agent/new.go", "content": "package agent"},
		},
		{
			name:     "absolute write inside scope is allowed",
			toolName: tools.WriteToolName,
			input:    map[string]string{"file_path": insideAbsolute, "content": "package agent"},
		},
		{
			name:     "edit inside scope is allowed",
			toolName: tools.EditToolName,
			input:    map[string]string{"file_path": "internal/agent/agent.go"},
		},
		{
			name:     "multiedit inside scope is allowed",
			toolName: tools.MultiEditToolName,
			input:    map[string]string{"file_path": "internal/agent/agent.go"},
		},
		{
			name:       "write outside scope is denied",
			toolName:   tools.WriteToolName,
			input:      map[string]string{"file_path": "internal/ui/model.go", "content": "package ui"},
			wantDenied: true,
		},
		{
			name:       "edit outside scope is denied",
			toolName:   tools.EditToolName,
			input:      map[string]string{"file_path": "../outside.go"},
			wantDenied: true,
		},
		{
			name:       "multiedit outside scope is denied",
			toolName:   tools.MultiEditToolName,
			input:      map[string]string{"file_path": "README.md"},
			wantDenied: true,
		},
		{
			name:       "prefix sibling is not inside scope",
			toolName:   tools.WriteToolName,
			input:      map[string]string{"file_path": "internal/agents/new.go", "content": "package agents"},
			wantDenied: true,
		},
		{
			name:       "missing path cannot be proven inside scope",
			toolName:   tools.WriteToolName,
			input:      map[string]string{"content": "package agent"},
			wantDenied: true,
		},
		{
			name:     "read-only tool is untouched",
			toolName: tools.ViewToolName,
			input:    map[string]string{"file_path": "internal/ui/model.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, ran := runScopeGuardedTool(
				t, ctx, dataDir, nil, workingDir, tc.toolName, tc.input,
			)
			if tc.wantDenied {
				require.True(t, resp.IsError, "expected denial, got %q", resp.Content)
				require.False(t, ran, "inner tool ran despite scope denial")
				require.Contains(t, resp.Content, job.ID)
				require.Contains(t, resp.Content, "declared scope")
			} else {
				require.False(t, resp.IsError, "unexpected denial: %q", resp.Content)
				require.True(t, ran, "inner tool did not run for allowed call")
			}
		})
	}
}

func TestScopeGuardInactiveJobsDoNotRestrict(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	ctx := planTestContext("session-1")

	t.Run("no jobs", func(t *testing.T) {
		resp, ran := runScopeGuardedTool(
			t, ctx, t.TempDir(), nil, workingDir, tools.WriteToolName,
			map[string]string{"file_path": "outside.go", "content": "package main"},
		)
		require.False(t, resp.IsError)
		require.True(t, ran)
	})

	t.Run("job without declared scope", func(t *testing.T) {
		dataDir := t.TempDir()
		createScopeTestJob(t, dataDir, nil)
		resp, ran := runScopeGuardedTool(
			t, ctx, dataDir, nil, workingDir, tools.WriteToolName,
			map[string]string{"file_path": "outside.go", "content": "package main"},
		)
		require.False(t, resp.IsError)
		require.True(t, ran)
	})

	t.Run("completed job", func(t *testing.T) {
		dataDir := t.TempDir()
		job := createScopeTestJob(t, dataDir, []string{"internal/agent"})
		markScopeTestJobComplete(t, dataDir, job)
		resp, ran := runScopeGuardedTool(
			t, ctx, dataDir, nil, workingDir, tools.WriteToolName,
			map[string]string{"file_path": "outside.go", "content": "package main"},
		)
		require.False(t, resp.IsError)
		require.True(t, ran)
	})
}

func TestScopeGuardPlanModeStillOwnsItsFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	dataDir := t.TempDir()
	createScopeTestJob(t, dataDir, []string{"internal/agent"})

	plans := plan.NewService(t.TempDir())
	entered, err := plans.Enter("session-1")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entered.Path, []byte("# Plan"), 0o644))

	resp, ran := runScopeGuardedTool(
		t, planTestContext("session-1"), dataDir, plans, workingDir,
		tools.WriteToolName,
		map[string]string{"file_path": entered.Path, "content": "# Plan"},
	)
	require.False(t, resp.IsError, "plan file write was blocked by scope guard: %q", resp.Content)
	require.True(t, ran)
}
