package deletionguard

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func backgroundFixture(t *testing.T) (*Guard, string, Classification, BackgroundJob) {
	t.Helper()
	guard, dir, c, _ := recoveryFixture(t)
	require.NoError(t, guard.BeginExecution("s", c, "launch"))
	job, err := guard.BindBackground("s", c, "launch", BackgroundJob{SessionID: "s", ToolCallID: "launch", ShellID: "001", ExecutionID: "unique-process-job", Command: "rm a b", WorkingDir: dir, WorkspaceDir: dir, Classification: c})
	require.NoError(t, err)
	require.Equal(t, c.Digest(), job.ApprovalSet)
	return guard, dir, c, job
}

func TestBackgroundTerminalOutcomesPreserveApproval(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   int
		detail string
	}{{"failed", 7, "exit status 7"}, {"cancelled", 1, "context canceled"}, {"timed out", 1, "context deadline exceeded"}} {
		t.Run(tc.name, func(t *testing.T) {
			guard, _, c, job := backgroundFixture(t)
			require.False(t, guard.Check(t.Context(), "s", "retry-before", c).Allowed)
			result := BackgroundResult{Job: job, Done: true, ExitCode: tc.code, Error: tc.detail}
			require.NoError(t, guard.ReconcileBackground("s", result))
			require.True(t, guard.Check(t.Context(), "s", "retry", c).Allowed)
			record, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Len(t, record.Confirmations, 2)
			require.Empty(t, record.Executions)
			require.Len(t, record.Attempts, 2)
			require.Equal(t, tc.detail, record.Attempts[1].BackgroundOutcome.Error)
			require.Equal(t, tc.code, record.Attempts[1].BackgroundOutcome.ExitCode)
			require.NoError(t, guard.ReconcileBackground("s", result))
			again, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Equal(t, record.revision, again.revision, "duplicate terminal evidence must not append records")
		})
	}
}

func TestBackgroundSuccessAndPartialFailureAfterRestart(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "all succeed", true: "partial fail"}[partial], func(t *testing.T) {
			guard, dir, c, job := backgroundFixture(t)
			require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
			if !partial {
				require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-b")))
			}
			guard = testGuard(guard.ledger.dataDir, nil)
			pending, err := guard.PendingBackground("s")
			require.NoError(t, err)
			require.Equal(t, []BackgroundJob{job}, pending)
			require.False(t, guard.Check(t.Context(), "s", "unknown-after-restart", c).Allowed)
			result := BackgroundResult{Job: job, Done: true}
			if partial {
				result.ExitCode = 9
				result.Error = "exit status 9"
			}
			require.NoError(t, guard.ReconcileBackground("s", result))
			record, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Len(t, record.Confirmations, 2)
			require.Len(t, record.Executions, 1)
			require.Equal(t, c.Digest(), record.Executions[0].ApprovalSet)
			require.False(t, guard.Check(t.Context(), "s", "repeat", Classify("rm a", dir)).Allowed)
			if partial {
				require.True(t, guard.Check(t.Context(), "s", "remainder", Classify("rm b", dir)).Allowed)
			} else {
				require.False(t, guard.Check(t.Context(), "s", "repeat-all", c).Allowed)
			}
			pending, err = guard.PendingBackground("s")
			require.NoError(t, err)
			require.Empty(t, pending)
		})
	}
}

func TestBackgroundStillRunningAndMissingEvidenceRemainBlocked(t *testing.T) {
	guard, _, c, job := backgroundFixture(t)
	before, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.NoError(t, guard.ReconcileBackground("s", BackgroundResult{Job: job, Done: false}))
	guard = testGuard(guard.ledger.dataDir, nil)
	require.False(t, guard.Check(t.Context(), "s", "retry", c).Allowed)
	after, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Equal(t, before.revision, after.revision)
	require.Empty(t, after.Executions)
	require.Len(t, after.Confirmations, 2)
}

func TestBackgroundRejectsMismatchedTerminalEvidence(t *testing.T) {
	for _, field := range []string{"session", "shell", "execution", "command", "working dir", "workspace", "call", "approval", "target"} {
		t.Run(field, func(t *testing.T) {
			guard, _, c, job := backgroundFixture(t)
			switch field {
			case "session":
				job.SessionID = "other"
			case "shell":
				job.ShellID = "002"
			case "execution":
				job.ExecutionID = "new-process-reused-shell-id"
			case "command":
				job.Command = "rm other"
			case "working dir":
				job.WorkingDir = "/other"
			case "workspace":
				job.WorkspaceDir = "/other"
			case "call":
				job.ToolCallID = "other"
			case "approval":
				job.ApprovalSet = "other"
			case "target":
				job.Classification.Targets = []string{"/other"}
			}
			require.Error(t, guard.ReconcileBackground("s", BackgroundResult{Job: job, Done: true}))
			require.False(t, guard.Check(t.Context(), "s", "retry", c).Allowed)
			record, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Len(t, record.Attempts, 1)
			require.Empty(t, record.Executions)
		})
	}
}

func TestBackgroundUncertainTerminalEvidenceIsNotReinterpreted(t *testing.T) {
	guard, dir, _, job := backgroundFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("unexpected changed bytes"), 0o600))
	result := BackgroundResult{Job: job, Done: true}
	require.NoError(t, guard.ReconcileBackground("s", result))
	before, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Equal(t, "uncertain", before.Attempts[len(before.Attempts)-1].State)
	require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
	require.NoError(t, guard.ReconcileBackground("s", result))
	after, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Equal(t, before.revision, after.revision)
	require.Empty(t, after.Executions)
	result.ExitCode = 1
	require.ErrorContains(t, guard.ReconcileBackground("s", result), "conflicting")
}

func TestBackgroundEvidenceBoundAndConflictPreservesOriginal(t *testing.T) {
	evidence := &backgroundEvidence{results: make(map[string]BackgroundResult), bindings: make(map[string]BackgroundJob), slots: make(map[string]bool)}
	for i := range maxPendingBackgroundOutcomes {
		require.NoError(t, evidence.reserve("s", strconv.Itoa(i)))
	}
	require.ErrorContains(t, evidence.reserve("s", "overflow"), "capacity")
	evidence.release("s", "0")
	require.NoError(t, evidence.reserve("s", "available"))
	original := BackgroundResult{Job: BackgroundJob{SessionID: "s", ExecutionID: "immutable", ToolCallID: "available", Classification: Classification{Targets: []string{"original"}}}, Done: true, Error: "original terminal error"}
	require.NoError(t, evidence.remember(original))
	original.Job.Classification.Targets[0] = "mutated input"
	stored := evidence.results[backgroundEvidenceKey(original)]
	require.Equal(t, []string{"original"}, stored.Job.Classification.Targets)
	require.ErrorContains(t, evidence.remember(original), "conflicting")
	require.Equal(t, []string{"original"}, evidence.results[backgroundEvidenceKey(original)].Job.Classification.Targets)
	// Forget is the single atomic handoff: a new reservation can only become
	// available after the saved result and binding have also left the queue.
	evidence.forget(stored)
	require.Empty(t, evidence.results)
	require.Len(t, evidence.slots, maxPendingBackgroundOutcomes-1)
	require.NoError(t, evidence.reserve("s", "replacement"))
}

func TestEmptyCombinedOutputBackgroundPostcondition(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	c := Classify("&> a", dir)
	require.True(t, guard.Check(t.Context(), "combined", "launch", c).Allowed)
	require.NoError(t, guard.BeginExecution("combined", c, "launch"))
	job, err := guard.BindBackground("combined", c, "launch", BackgroundJob{SessionID: "combined", ToolCallID: "launch", ShellID: "001", ExecutionID: "combined-output", Command: "&> a", WorkingDir: dir, WorkspaceDir: dir, Classification: c})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), nil, 0o600))
	require.NoError(t, guard.ReconcileBackground("combined", BackgroundResult{Job: job, Done: true}))
	record, err := guard.Ledger().Read("combined")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, "finished", record.Attempts[1].State)
}

func TestBackgroundEvidenceSharesOnlyExactAuthorityAndLedger(t *testing.T) {
	dir := t.TempDir()
	first := &fixtureAuthority{data: make(map[string][]byte), revisions: make(map[string]uint64)}
	second := &fixtureAuthority{data: make(map[string][]byte), revisions: make(map[string]uint64)}
	a := NewGuardWithAuthority(dir, nil, first)
	restarted := NewGuardWithAuthority(dir, nil, first)
	foreign := NewGuardWithAuthority(dir, nil, second)
	require.Same(t, a.ledger.background, restarted.ledger.background)
	require.NotSame(t, a.ledger.background, foreign.ledger.background)
	require.NotSame(t, a.ledger.background, NewGuardWithAuthority(t.TempDir(), nil, first).ledger.background)
}
