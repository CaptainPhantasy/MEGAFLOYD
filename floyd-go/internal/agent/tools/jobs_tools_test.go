package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runJobTool(t *testing.T, tool fantasy.AgentTool, ctx context.Context, params any) fantasy.ToolResponse {
	t.Helper()
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "jobs-test-call",
		Name:  tool.Info().Name,
		Input: string(input),
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	return resp
}

func decodeContract(t *testing.T, resp fantasy.ToolResponse) ContractResponse {
	t.Helper()
	var out ContractResponse
	require.NotEmpty(t, resp.Metadata)
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &out))
	return out
}

func TestContractToolRefusesCompletionWithoutEvidence(t *testing.T) {
	t.Parallel()
	workingDir := t.TempDir()
	artifact := filepath.Join(workingDir, "app.go")
	require.NoError(t, os.WriteFile(artifact, []byte("v1"), 0o644))
	dataDir := filepath.Join(workingDir, ".floyd")
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	permissions := &mockPermissionService{}
	contract := NewContractTool(dataDir, permissions, workingDir)
	prove := NewProveTool(dataDir, permissions, workingDir)

	created := runJobTool(t, contract, ctx, ContractParams{
		Action:       "create",
		Objective:    "Ship the job system",
		Requirements: []string{"Tests pass"},
		Artifact:     "app.go",
		Scope:        []string{"internal/jobs"},
	})
	meta := decodeContract(t, created)
	require.NotEmpty(t, meta.JobID)
	assert.Contains(t, created.Content, "Contract created")

	refused := runJobTool(t, contract, ctx, ContractParams{Action: "complete", JobID: meta.JobID})
	assert.Contains(t, refused.Content, "Completion refused")

	runJobTool(t, prove, ctx, ProveParams{
		Action:      "record",
		JobID:       meta.JobID,
		Requirement: "tests-pass",
		Command:     "go test ./internal/jobs",
		Output:      "ok",
		ExitCode:    0,
	})

	completed := runJobTool(t, contract, ctx, ContractParams{Action: "complete", JobID: meta.JobID})
	assert.Contains(t, completed.Content, "marked complete")
	assert.True(t, decodeContract(t, completed).Complete)
}

func TestProveToolInvalidatesOnArtifactChange(t *testing.T) {
	t.Parallel()
	workingDir := t.TempDir()
	artifact := filepath.Join(workingDir, "app.go")
	require.NoError(t, os.WriteFile(artifact, []byte("v1"), 0o644))
	dataDir := filepath.Join(workingDir, ".floyd")
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	permissions := &mockPermissionService{}
	contract := NewContractTool(dataDir, permissions, workingDir)
	prove := NewProveTool(dataDir, permissions, workingDir)

	meta := decodeContract(t, runJobTool(t, contract, ctx, ContractParams{
		Action:       "create",
		Objective:    "Prove invalidation",
		Requirements: []string{"Tests pass"},
		Artifact:     "app.go",
	}))
	runJobTool(t, prove, ctx, ProveParams{
		Action: "record", JobID: meta.JobID, Requirement: "tests-pass",
		Command: "go test ./internal/jobs", Output: "ok",
	})

	report, err := jobs.CheckCompletion(dataDir, meta.JobID)
	require.NoError(t, err)
	assert.True(t, report.Complete)

	require.NoError(t, os.WriteFile(artifact, []byte("v2"), 0o644))
	report, err = jobs.CheckCompletion(dataDir, meta.JobID)
	require.NoError(t, err)
	assert.False(t, report.Complete)

	shown := runJobTool(t, prove, ctx, ProveParams{Action: "show", JobID: meta.JobID})
	assert.Contains(t, shown.Content, "artifact changed")
}

func TestWhyToolSaysUnknown(t *testing.T) {
	t.Parallel()
	workingDir := t.TempDir()
	dataDir := filepath.Join(workingDir, ".floyd")
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	contract := NewContractTool(dataDir, &mockPermissionService{}, workingDir)

	meta := decodeContract(t, runJobTool(t, contract, ctx, ContractParams{
		Action: "create", Objective: "Ask why", Requirements: []string{"It runs"},
	}))
	why := NewWhyTool(dataDir, workingDir)
	resp := runJobTool(t, why, ctx, WhyParams{JobID: meta.JobID, ActionID: "j-missing"})
	assert.Contains(t, resp.Content, "UNKNOWN")
	assert.NotContains(t, resp.Content, "RECORDED")
}

func TestDriftToolReportsOutOfScopeChange(t *testing.T) {
	t.Parallel()
	workingDir := t.TempDir()
	dataDir := filepath.Join(workingDir, ".floyd")
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	contract := NewContractTool(dataDir, &mockPermissionService{}, workingDir)

	meta := decodeContract(t, runJobTool(t, contract, ctx, ContractParams{
		Action:       "create",
		Objective:    "Keep scope",
		Requirements: []string{"It runs"},
		Scope:        []string{"internal/jobs"},
	}))
	drift := NewDriftTool(dataDir, workingDir)
	resp := runJobTool(t, drift, ctx, DriftParams{
		JobID: meta.JobID,
		Paths: []string{"internal/jobs/jobs.go", "internal/agent/coordinator.go"},
	})
	assert.Contains(t, resp.Content, "Violations:")
	assert.Contains(t, resp.Content, "internal/agent/coordinator.go")
	assert.NotContains(t, resp.Content, "- internal/jobs/jobs.go")
}

func TestReplayHandoffAndRecoverTools(t *testing.T) {
	t.Parallel()
	workingDir := t.TempDir()
	artifact := filepath.Join(workingDir, "app.go")
	require.NoError(t, os.WriteFile(artifact, []byte("v1"), 0o644))
	dataDir := filepath.Join(workingDir, ".floyd")
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
	permissions := &mockPermissionService{}
	contract := NewContractTool(dataDir, permissions, workingDir)
	prove := NewProveTool(dataDir, permissions, workingDir)

	meta := decodeContract(t, runJobTool(t, contract, ctx, ContractParams{
		Action:       "create",
		Objective:    "Hand off this job",
		Requirements: []string{"Tests pass"},
		Artifact:     "app.go",
	}))
	runJobTool(t, prove, ctx, ProveParams{
		Action: "record", JobID: meta.JobID, Requirement: "tests-pass",
		Command: "go test ./internal/jobs", Output: "ok",
	})

	replay := NewReplayTool(dataDir, workingDir)
	replayed := runJobTool(t, replay, ctx, ReplayParams{JobID: meta.JobID})
	assert.Contains(t, replayed.Content, "No commands are executed")
	assert.Contains(t, replayed.Content, "go test ./internal/jobs")

	handoff := NewHandoffTool(dataDir, permissions, workingDir)
	packet := runJobTool(t, handoff, ctx, HandoffParams{JobID: meta.JobID})
	assert.Contains(t, packet.Content, "Handoff written")
	assert.FileExists(t, filepath.Join(dataDir, "jobs", meta.JobID, "handoff.md"))

	recoverTool := NewRecoverTool(dataDir, workingDir)
	recovered := runJobTool(t, recoverTool, ctx, RecoverParams{})
	assert.Contains(t, recovered.Content, meta.JobID)
	assert.Contains(t, recovered.Content, "Mark the contract complete")
}

func TestJobsToolsRegistration(t *testing.T) {
	t.Parallel()
	tools := JobsTools(t.TempDir(), nil, t.TempDir())
	require.Len(t, tools, 7)
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}
	assert.ElementsMatch(t, []string{
		ContractToolName,
		ProveToolName,
		DriftToolName,
		WhyToolName,
		RecoverToolName,
		HandoffToolName,
		ReplayToolName,
	}, names)
}
