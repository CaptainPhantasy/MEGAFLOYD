package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/shell"
	"github.com/stretchr/testify/require"
)

func hookDeletionTool(t *testing.T, f *deletionFixture, rewritten string, body fantasy.AgentTool) fantasy.AgentTool {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"decision": "allow", "updated_input": map[string]string{"command": rewritten, "description": "safe recording regression"}})
	require.NoError(t, err)
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: f.dataDir}, Hooks: map[string][]config.HookConfig{hooks.EventPreToolUse: {{Command: "printf '%s\\n' '" + string(payload) + "'"}}}}, f.workingDir)
	require.NoError(t, cfg.Config().ValidateHooks())
	runtime := hookruntime.New(t.Context(), cfg, nil)
	t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })
	c := &coordinator{cfg: cfg, hookRuntime: runtime}
	tool := c.wrapAgentTools([]fantasy.AgentTool{body}, false)[0]
	// Replace only the authority fixture, retaining the actual coordinator ordering.
	var setFixture func(fantasy.AgentTool)
	setFixture = func(current fantasy.AgentTool) {
		switch wrapped := current.(type) {
		case *runtimeObservedTool:
			setFixture(wrapped.AgentTool)
		case *planGuardTool:
			setFixture(wrapped.inner)
		case *scopeGuardTool:
			setFixture(wrapped.inner)
		case *runtimePreTool:
			setFixture(wrapped.AgentTool)
		case *deletionGuardTool:
			wrapped.guard = f.guard
		default:
			t.Fatalf("deletion guard missing from coordinator tool composition: %T", current)
		}
	}
	setFixture(tool)
	return tool
}

func hookDeletionRecordingBody(ran *bool, actual *string) fantasy.AgentTool {
	return fantasy.NewAgentTool(tools.BashToolName, "record only", func(_ context.Context, p tools.BashParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		*ran = true
		*actual = p.Command
		zero := 0
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("recorded"), tools.BashResponseMetadata{ExitCode: &zero}), nil
	})
}

func TestRuntimeHookDeletionRewriteRequiresBothConfirmations(t *testing.T) {
	for _, answers := range [][]bool{nil, {true}, {true, true}} {
		t.Run(map[int]string{0: "none", 1: "one", 2: "two"}[len(answers)], func(t *testing.T) {
			f := newDeletionFixture(t)
			f.answers = answers
			ran, actual := false, ""
			tool := hookDeletionTool(t, f, "rm a", hookDeletionRecordingBody(&ran, &actual))
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "rewrite")
			response, err := tool.Run(ctx, fantasy.ToolCall{ID: "call", Name: tools.BashToolName, Input: `{"command":"printf harmless"}`})
			require.NoError(t, err)
			require.Equal(t, len(answers) == 2, ran, response.Content)
			require.Equal(t, len(answers) != 2, response.IsError, response.Content)
			if ran {
				require.Equal(t, "rm a", actual)
				require.Len(t, f.record("rewrite").Confirmations, 2)
			}
		})
	}
}

func TestRuntimeHookDeletionRewriteDoesNotReuseOtherTargetApproval(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	require.True(t, f.guard.Check(t.Context(), "rewrite-target", "approved", deletionguard.Classify("rm a", f.workingDir)).Allowed)
	ran, actual := false, ""
	tool := hookDeletionTool(t, f, "rm b", hookDeletionRecordingBody(&ran, &actual))
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "rewrite-target")
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "call", Name: tools.BashToolName, Input: `{"command":"rm a"}`})
	require.NoError(t, err)
	require.True(t, response.IsError, response.Content)
	require.False(t, ran)
	require.Len(t, f.record("rewrite-target").Confirmations, 2)
	require.Empty(t, f.record("rewrite-target").Attempts)
	require.Equal(t, []string{f.target("b")}, f.asked[len(f.asked)-1].Targets)
}

func TestRuntimeHookDeletionHarmlessRewriteRemainsUsable(t *testing.T) {
	f := newDeletionFixture(t)
	ran, actual := false, ""
	tool := hookDeletionTool(t, f, "printf rewritten", hookDeletionRecordingBody(&ran, &actual))
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "harmless")
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "call", Name: tools.BashToolName, Input: `{"command":"printf original"}`})
	require.NoError(t, err)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Equal(t, "printf rewritten", actual)
	require.Empty(t, f.asked)
}

func TestRuntimeHookDeletionBackgroundBindsActualRewrittenCommand(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	rewritten := "sleep 0.02; if false; then rm a; fi; false"
	var job *shell.BackgroundShell
	body := fantasy.NewAgentTool(tools.BashToolName, "safe background fixture", func(ctx context.Context, p tools.BashParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		var err error
		job, err = shell.GetBackgroundShellManager().StartForSession(context.Background(), f.workingDir, nil, p.Command, "safe rewrite fixture", tools.GetSessionFromContext(ctx), f.workingDir)
		if err != nil {
			return fantasy.ToolResponse{}, err
		}
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse("started"), tools.BashResponseMetadata{Background: true, ShellID: job.ID}), nil
	})
	tool := hookDeletionTool(t, f, rewritten, body)
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "rewrite-background")
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "launch", Name: tools.BashToolName, Input: `{"command":"printf original"}`})
	require.NoError(t, err)
	require.False(t, response.IsError, response.Content)
	require.NotNil(t, job)
	t.Cleanup(func() { job.Wait(); require.NoError(t, shell.GetBackgroundShellManager().Remove(job.ID)) })
	record := f.record("rewrite-background")
	require.Len(t, record.BackgroundJobs, 1)
	require.Equal(t, rewritten, record.BackgroundJobs[0].Command)
	require.Equal(t, job.ExecutionID, record.BackgroundJobs[0].ExecutionID)
	require.Equal(t, []string{f.target("a")}, record.BackgroundJobs[0].Classification.Targets)
	require.Len(t, record.Confirmations, 2)
	require.Eventually(t, func() bool {
		record, err := f.guard.Ledger().Read("rewrite-background")
		return err == nil && len(record.Attempts) == 2
	}, time.Second, time.Millisecond)
	require.True(t, f.guard.Check(t.Context(), "rewrite-background", "retry", deletionguard.Classify(rewritten, f.workingDir)).Allowed)
	require.Empty(t, f.record("rewrite-background").Executions)
}
