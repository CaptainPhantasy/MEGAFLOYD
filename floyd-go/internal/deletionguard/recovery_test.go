package deletionguard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recoveryConfirmer struct{ calls int }

func (c *recoveryConfirmer) Confirm(context.Context, ConfirmationRequest) (bool, error) {
	c.calls++
	return true, nil
}

func TestRecoveryExactRemainderAfterRestart(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(a, []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(b, []byte("b"), 0o600))
	confirmer := &recoveryConfirmer{}
	guard := testGuard(data, confirmer)
	require.True(t, guard.Check(t.Context(), "s", "one", Classify("rm a b", dir)).Allowed)
	require.Equal(t, 2, confirmer.calls)
	require.NoError(t, guard.RecordExecution("s", Classify("rm a", dir), "one"))
	require.True(t, testGuard(data, nil).Check(t.Context(), "s", "two", Classify("rm b", dir)).Allowed, "unchanged remainder retains original answers")
}

func recoveryFixture(t *testing.T) (*Guard, string, Classification, *recoveryConfirmer) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	confirmer := &recoveryConfirmer{}
	guard := testGuard(t.TempDir(), confirmer)
	all := Classify("rm a b", dir)
	require.True(t, guard.Check(t.Context(), "s", "one", all).Allowed)
	return guard, dir, all, confirmer
}

func TestRecoveryRecordsCompletedOriginal(t *testing.T) {
	guard, dir, all, c := recoveryFixture(t)
	require.NoError(t, guard.BeginExecution("s", all, "one"))
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.Rename(filepath.Join(dir, name), filepath.Join(dir, "quarantined-"+name)))
	}
	require.NoError(t, guard.RecordOutcome("s", all, "one", all.Targets, false))
	require.False(t, guard.Check(t.Context(), "s", "two", all).Allowed)
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Len(t, record.Confirmations, 2)
	require.Len(t, record.Executions, 1)
	require.Equal(t, all.Digest(), record.Executions[0].ApprovalSet)
	require.Equal(t, 2, c.calls)
}

func TestRecoveryReconcilesFailedRemovalByObservation(t *testing.T) {
	guard, dir, all, c := recoveryFixture(t)
	require.NoError(t, guard.BeginExecution("s", all, "one"))
	// Quarantine simulates a removed pathname without destroying fixture bytes.
	require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
	require.NoError(t, guard.RecordOutcome("s", all, "one", nil, false))
	guard = testGuard(guard.ledger.dataDir, nil)
	remaining := Classify("rm b", dir)
	require.True(t, guard.Check(t.Context(), "s", "two", remaining).Allowed)
	require.NoError(t, guard.BeginExecution("s", remaining, "two"))
	require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-b")))
	require.NoError(t, guard.RecordOutcome("s", remaining, "two", remaining.Targets, false))
	require.False(t, guard.Check(t.Context(), "s", "three", remaining).Allowed)
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Len(t, record.Confirmations, 2)
	require.Len(t, record.Executions, 2)
	require.Len(t, record.Attempts, 4)
	require.Equal(t, all.Digest(), record.Executions[0].ApprovalSet)
	require.Equal(t, all.Digest(), record.Executions[1].ApprovalSet)
	require.Equal(t, 2, c.calls)
}

func TestRecoveryRejectsChangedAndBroaderRemainders(t *testing.T) {
	for _, mode := range []string{"added", "substituted", "completed", "repeat-original", "changed", "replaced", "different-operation", "recursive"} {
		t.Run(mode, func(t *testing.T) {
			guard, dir, all, c := recoveryFixture(t)
			require.NoError(t, guard.BeginExecution("s", all, "one"))
			require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
			require.NoError(t, guard.RecordOutcome("s", all, "one", Classify("rm a", dir).Targets, false))
			guard.SetConfirmer(nil)
			command := "rm b"
			switch mode {
			case "added":
				command = "rm b c"
			case "substituted":
				command = "rm c"
			case "completed":
				command = "rm a"
			case "repeat-original":
				command = "rm a b"
			case "changed":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("changed"), 0o600))
			case "replaced":
				require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "old-b")))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("b"), 0o600))
			case "different-operation":
				command = "truncate -s 0 b"
			case "recursive":
				command = "rm -r b"
			}
			require.False(t, guard.Check(t.Context(), "s", "two", Classify(command, dir)).Allowed)
			require.Equal(t, 2, c.calls)
		})
	}
}

func TestRecoveryUncertainExecutionStaysBlockedAfterRestart(t *testing.T) {
	for _, outcome := range []string{"crash", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			guard, dir, all, _ := recoveryFixture(t)
			require.NoError(t, guard.BeginExecution("s", all, "one"))
			if outcome == "uncertain" {
				require.NoError(t, guard.RecordOutcome("s", all, "one", nil, true))
			}
			guard = testGuard(guard.ledger.dataDir, nil)
			require.False(t, guard.Check(t.Context(), "s", "two", Classify("rm b", dir)).Allowed)
			require.False(t, guard.Check(t.Context(), "s", "two", all).Allowed)
			record, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
			require.Len(t, record.Confirmations, 2)
		})
	}
}

func TestRecoveryRecursiveRemainderIdentity(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	for _, name := range []string{"left", "right"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "child"), []byte("data"), 0o600))
	}
	all := Classify("rm -r left right", dir)
	require.True(t, guard.Check(t.Context(), "tree", "one", all).Allowed)
	require.NoError(t, guard.BeginExecution("tree", all, "one"))
	require.NoError(t, os.Rename(filepath.Join(dir, "left"), filepath.Join(dir, "quarantined-left")))
	require.NoError(t, guard.RecordOutcome("tree", all, "one", Classify("rm -r left", dir).Targets, false))
	guard.SetConfirmer(nil)
	remaining := Classify("rm -r right", dir)
	require.True(t, guard.Check(t.Context(), "tree", "two", remaining).Allowed)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "right", "child"), []byte("new data"), 0o600))
	require.False(t, guard.Check(t.Context(), "tree", "two", remaining).Allowed)
}

func TestRecoverySkippedRemovalIsNotCompleted(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	c := Classify("if false; then rm b; fi", dir)
	require.True(t, guard.Check(t.Context(), "skip", "one", c).Allowed)
	require.NoError(t, guard.BeginExecution("skip", c, "one"))
	require.NoError(t, guard.RecordOutcome("skip", c, "one", c.Targets, false))
	record, err := guard.Ledger().Read("skip")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.True(t, guard.Check(t.Context(), "skip", "two", Classify("rm b", dir)).Allowed)
}

type changingRecoveryConfirmer struct {
	path  string
	calls int
}

func (c *changingRecoveryConfirmer) Confirm(context.Context, ConfirmationRequest) (bool, error) {
	c.calls++
	if c.calls == 1 {
		return true, os.WriteFile(c.path, []byte("changed during confirmation"), 0o600)
	}
	return true, nil
}

func TestRecoveryTargetChangedBetweenConfirmations(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	confirmer := &changingRecoveryConfirmer{path: path}
	guard := testGuard(data, confirmer)
	classification := Classify("rm a", dir)
	require.False(t, guard.Check(t.Context(), "s", "one", classification).Allowed)
	require.Equal(t, 1, confirmer.calls)
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Len(t, record.Confirmations, 1)
	require.False(t, testGuard(data, confirmer).Check(t.Context(), "s", "two", classification).Allowed)
	require.Equal(t, 1, confirmer.calls)
}

func TestRecoverySeparateGuardsCannotLaunchSameApproval(t *testing.T) {
	guard, _, all, _ := recoveryFixture(t)
	second := testGuard(guard.ledger.dataDir, nil)
	require.True(t, second.Check(t.Context(), "s", "two", all).Allowed)
	require.NoError(t, guard.BeginExecution("s", all, "one"))
	require.Error(t, second.BeginExecution("s", all, "two"))
	require.NoError(t, guard.RecordOutcome("s", all, "one", nil, false))
	require.Error(t, guard.RecordOutcome("s", all, "one", nil, false))
	require.Error(t, guard.RecordOutcome("s", all, "unlaunched", nil, false))
}

func TestRecoverySkippedTruncateIsNotCompleted(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	c := Classify("if false; then truncate -s 0 b; fi", dir)
	require.True(t, guard.Check(t.Context(), "skip", "one", c).Allowed)
	require.NoError(t, guard.BeginExecution("skip", c, "one"))
	require.NoError(t, guard.RecordOutcome("skip", c, "one", c.Targets, false))
	record, err := guard.Ledger().Read("skip")
	require.NoError(t, err)
	require.Empty(t, record.Executions, "unchanged bytes prove no truncation was completed")
}

func TestRecoveryBindsTruncateSizeAndMixedOperations(t *testing.T) {
	for _, command := range []string{"truncate -s 5 a b", "rm a; shred b"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			all := Classify(command, dir)
			require.True(t, guard.Check(t.Context(), "operations", "one", all).Allowed)
			require.NoError(t, guard.BeginExecution("operations", all, "one"))
			require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "quarantined-a")))
			if command == "truncate -s 5 a b" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("12345"), 0o600))
			}
			require.NoError(t, guard.RecordOutcome("operations", all, "one", []string{filepath.Join(dir, "a")}, false))
			guard = testGuard(guard.ledger.dataDir, nil)
			retry, changed := "truncate -s 5 b", "truncate -s 0 b"
			if command == "rm a; shred b" {
				retry, changed = "shred b", "rm b"
			}
			require.True(t, guard.Check(t.Context(), "operations", "two", Classify(retry, dir)).Allowed)
			require.False(t, guard.Check(t.Context(), "operations", "two", Classify(changed, dir)).Allowed)
		})
	}
}

func TestRecoveryConfirmationOnlyHistoryCannotBindReplacement(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	target := filepath.Join(dir, "a")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o600))
	guard := testGuard(data, nil)
	classification := Classify("rm a", dir)
	for stage := 1; stage <= 2; stage++ {
		require.NoError(t, guard.Ledger().AppendConfirmation("s", Confirmation{Stage: stage, TargetSet: classification.Digest(), Targets: classification.Targets, Source: humanSource}))
	}
	require.NoError(t, os.Rename(target, filepath.Join(dir, "quarantined-original")))
	require.NoError(t, os.WriteFile(target, []byte("replacement"), 0o600))
	require.False(t, guard.Check(t.Context(), "s", "call", classification).Allowed, "unbound answers cannot approve a newly observed inode")
	require.Error(t, guard.BeginExecution("s", classification, "call"), "launch cannot synthesize approval for a replacement")
}

func TestRecoveryRevalidatesBoundApprovalBeforeLaunch(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	require.True(t, guard.Check(t.Context(), "s", "checked", all).Allowed)
	require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-original-b")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("replacement"), 0o600))
	require.Error(t, guard.BeginExecution("s", all, "changed-between-check-and-launch"))
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Empty(t, record.Executions)
	require.Len(t, record.Confirmations, 2)
}

func TestRecoveryLaunchRejectsReplacementDuringPreparation(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	originalClock := guard.now
	guard.now = func() time.Time {
		require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-before-launch")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("replacement before durable launch"), 0o600))
		return originalClock()
	}
	require.Error(t, guard.BeginExecution("s", all, "replacement-during-launch-preparation"))
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Empty(t, record.Executions)
}

func TestRecoveryRequiresCompleteCurrentApprovalBinding(t *testing.T) {
	for _, field := range []string{"snapshots", "operations", "target-set"} {
		t.Run(field, func(t *testing.T) {
			guard, _, all, _ := recoveryFixture(t)
			record, err := guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Len(t, record.Approvals, 1)
			switch field {
			case "snapshots":
				record.Approvals[0].Snapshots = nil
			case "operations":
				record.Approvals[0].Operations = nil
			case "target-set":
				record.Approvals[0].TargetSet = "unbound-identity"
			}
			require.NoError(t, guard.Ledger().write("s", record))
			require.False(t, guard.Check(t.Context(), "s", "check", all).Allowed)
			require.Error(t, guard.BeginExecution("s", all, "launch"))
			record, err = guard.Ledger().Read("s")
			require.NoError(t, err)
			require.Empty(t, record.Attempts)
			require.Len(t, record.Confirmations, 2)
		})
	}
}
