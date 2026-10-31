package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/permission"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/shell"
	"github.com/stretchr/testify/require"
)

func TestDeletionBackgroundTerminalFailureReusesApproval(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	require.NoError(t, os.WriteFile(f.target("a"), []byte("preserved"), 0o600))
	command := "if false; then rm a; fi; false"
	inner := fantasy.NewAgentTool(tools.BashToolName, "background fixture", func(ctx context.Context, params tools.BashParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		job, err := shell.GetBackgroundShellManager().StartForSession(context.Background(), f.workingDir, nil, params.Command, "fixture", tools.GetSessionFromContext(ctx), f.workingDir)
		require.NoError(t, err)
		job.Wait() // no deletion runs: the destructive branch is never entered
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("background fixture"), tools.BashResponseMetadata{Background: true, ShellID: job.ID, WorkingDirectory: f.workingDir}), nil
	})
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "background-call", Name: tools.BashToolName, Input: `{"command":"if false; then rm a; fi; false"}`})
	require.NoError(t, err)
	require.False(t, response.IsError, response.Content)
	// The next tool call reconciles terminal manager evidence before Check.
	response, ran := f.run("s", command)
	require.True(t, ran, response.Content)
	require.False(t, response.IsError, response.Content)
	require.Len(t, f.asked, 2)
	require.Len(t, f.record("s").Confirmations, 2)
	require.Empty(t, f.record("s").Executions)
}

func TestDeletionBackgroundActualBashAndJobOutput(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "explicit"}[explicit], func(t *testing.T) {
			f := newDeletionFixture(t)
			f.answers = []bool{true, true}
			require.NoError(t, os.WriteFile(f.target("a"), []byte("preserved"), 0o600))
			inner := tools.NewBashTool(permission.NewPermissionService(f.workingDir, true, nil), f.workingDir, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
			wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner, tools.NewJobOutputTool()}, f.guard, f.workingDir)
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
			params := tools.BashParams{Command: "sleep 1.2; if false; then rm a; fi; false", Description: "safe background regression", AutoBackgroundAfter: 1, RunInBackground: explicit}
			input, err := json.Marshal(params)
			require.NoError(t, err)
			resp, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: "actual-background", Name: tools.BashToolName, Input: string(input)})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			var meta tools.BashResponseMetadata
			require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
			require.True(t, meta.Background)
			require.NotEmpty(t, meta.ShellID)
			pending, err := f.guard.PendingBackground("s")
			require.NoError(t, err)
			require.Len(t, pending, 1)
			job, ok := shell.GetBackgroundShellManager().Get(meta.ShellID)
			require.True(t, ok)
			require.Equal(t, job.ExecutionID, pending[0].ExecutionID)
			require.False(t, f.guard.Check(t.Context(), "s", "premature", deletionguard.Classify(params.Command, f.workingDir)).Allowed)
			resp, err = wrapped[1].Run(ctx, fantasy.ToolCall{ID: "read-job", Name: tools.JobOutputToolName, Input: fmt.Sprintf(`{"shell_id":%q,"wait":true}`, meta.ShellID)})
			require.NoError(t, err)
			require.False(t, resp.IsError, resp.Content)
			require.Contains(t, resp.Content, "completed")
			require.True(t, f.guard.Check(t.Context(), "s", "retry", deletionguard.Classify(params.Command, f.workingDir)).Allowed)
			record := f.record("s")
			require.Len(t, record.Confirmations, 2)
			require.Empty(t, record.Executions)
			require.Len(t, record.Attempts, 2)
			require.NotEmpty(t, record.Attempts[1].BackgroundOutcome.Error)
			require.NoError(t, shell.GetBackgroundShellManager().Remove(meta.ShellID))
		})
	}
}

func TestDeletionBackgroundKillRetainsTerminalEvidence(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	require.NoError(t, os.WriteFile(f.target("a"), []byte("preserved"), 0o600))
	inner := tools.NewBashTool(permission.NewPermissionService(f.workingDir, true, nil), f.workingDir, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner, tools.NewJobKillTool()}, f.guard, f.workingDir)
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	command := "sleep 30; if false; then rm a; fi"
	input, err := json.Marshal(tools.BashParams{Command: command, RunInBackground: true})
	require.NoError(t, err)
	resp, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: "launch", Name: tools.BashToolName, Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	var meta tools.BashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	resp, err = wrapped[1].Run(ctx, fantasy.ToolCall{ID: "cancel", Name: tools.JobKillToolName, Input: fmt.Sprintf(`{"shell_id":%q}`, meta.ShellID)})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	_, exists := shell.GetBackgroundShellManager().Get(meta.ShellID)
	require.False(t, exists)
	require.Eventually(t, func() bool { r, e := f.guard.Ledger().Read("s"); return e == nil && len(r.Attempts) == 2 }, time.Second, time.Millisecond)
	require.True(t, f.guard.Check(t.Context(), "s", "retry", deletionguard.Classify(command, f.workingDir)).Allowed)
	record := f.record("s")
	require.Empty(t, record.Executions)
	require.Len(t, record.Confirmations, 2)
	require.NotEmpty(t, record.Attempts[1].BackgroundOutcome.Error)
}

func TestDeletionBackgroundRestartMissingOrReusedJobStaysBlocked(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	command := "if false; then rm a; fi"
	c := deletionguard.Classify(command, f.workingDir)
	require.True(t, f.guard.Check(t.Context(), "s", "launch", c).Allowed)
	require.NoError(t, f.guard.BeginExecution("s", c, "launch"))
	job, err := shell.GetBackgroundShellManager().StartForSession(t.Context(), f.workingDir, nil, command, "fixture", "s", f.workingDir)
	require.NoError(t, err)
	job.Wait()
	ref, err := f.guard.BindBackground("s", c, "launch", deletionguard.BackgroundJob{SessionID: "s", ToolCallID: "launch", ShellID: job.ID, ExecutionID: "prior-process-job", Command: command, WorkingDir: f.workingDir, WorkspaceDir: f.workingDir, Classification: c})
	require.NoError(t, err)
	f.guard = deletionguard.NewGuardWithAuthority(f.dataDir, nil, f.authority)
	wrapper := &deletionGuardTool{guard: f.guard, workingDir: f.workingDir}
	require.NoError(t, wrapper.reconcileBackground("s"))
	require.False(t, f.guard.Check(t.Context(), "s", "retry-reused", c).Allowed, "same shell ID with different execution UUID is not evidence")
	require.NoError(t, shell.GetBackgroundShellManager().Remove(job.ID))
	require.NoError(t, wrapper.reconcileBackground("s"))
	require.False(t, f.guard.Check(t.Context(), "s", "retry-missing", c).Allowed)
	pending, err := f.guard.PendingBackground("s")
	require.NoError(t, err)
	require.Equal(t, []deletionguard.BackgroundJob{ref}, pending)
	require.Empty(t, f.record("s").Executions)
}

type transientBackgroundAuthority struct {
	deletionguard.Authority
	failWrites atomic.Bool
	failed     chan struct{}
}

func (a *transientBackgroundAuthority) CompareAndSwap(ctx context.Context, namespace, session string, revision uint64, data []byte) error {
	if a.failWrites.Load() {
		select {
		case a.failed <- struct{}{}:
		default:
		}
		return errors.New("isolated transient authority outage")
	}
	return a.Authority.CompareAndSwap(ctx, namespace, session, revision, data)
}

func TestDeletionBackgroundRetryRetainsKilledJobAfterPersistenceFailure(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	require.NoError(t, os.WriteFile(f.target("a"), []byte("preserved"), 0o600))
	authority := &transientBackgroundAuthority{Authority: f.authority, failed: make(chan struct{}, 1)}
	f.guard = deletionguard.NewGuardWithAuthority(f.dataDir, f, authority)
	inner := tools.NewBashTool(permission.NewPermissionService(f.workingDir, true, nil), f.workingDir, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner, tools.NewJobKillTool()}, f.guard, f.workingDir)
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	command := "sleep 30; if false; then rm a; fi"
	input, err := json.Marshal(tools.BashParams{Command: command, RunInBackground: true})
	require.NoError(t, err)
	resp, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: "launch", Name: tools.BashToolName, Input: string(input)})
	require.NoError(t, err)
	require.False(t, resp.IsError, resp.Content)
	var meta tools.BashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	authority.failWrites.Store(true)
	_, _ = wrapped[1].Run(ctx, fantasy.ToolCall{ID: "cancel", Name: tools.JobKillToolName, Input: fmt.Sprintf(`{"shell_id":%q}`, meta.ShellID)})
	select {
	case <-authority.failed:
	case <-time.After(time.Second):
		t.Fatal("terminal persistence failure was not reproduced")
	}
	_, exists := shell.GetBackgroundShellManager().Get(meta.ShellID)
	require.False(t, exists)
	require.Len(t, f.record("s").Attempts, 1)
	authority.failWrites.Store(false)
	// A reconstructed Guard in the same engine process shares trusted retained
	// evidence; no workspace file or model-visible output supplies completion.
	f.guard = deletionguard.NewGuardWithAuthority(f.dataDir, nil, authority)
	resp, ran := f.run("s", command)
	require.True(t, ran, resp.Content)
	require.False(t, resp.IsError, resp.Content)
	record := f.record("s")
	require.Len(t, record.Confirmations, 2)
	require.Len(t, f.asked, 2)
	require.Empty(t, record.Executions)
	require.Len(t, record.Attempts, 4)
	require.NotNil(t, record.Attempts[1].BackgroundOutcome)
	require.NotEmpty(t, record.Attempts[1].BackgroundOutcome.Error)
}
