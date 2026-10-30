package deletionguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMoveRecoveryProvesReplacementAndPreservesRemainder(t *testing.T) {
	guard, dir, _, confirmer := recoveryFixture(t)
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, []byte("replacement"), 0o600))
	all := Classify("mv source b; rm a", dir)
	require.True(t, guard.Check(t.Context(), "move", "one", all).Allowed)
	require.NoError(t, guard.BeginExecution("move", all, "one"))
	require.NoError(t, os.Rename(destination, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, os.Rename(source, destination))
	require.NoError(t, guard.RecordOutcome("move", all, "one", []string{destination}, false))
	record, err := guard.Ledger().Read("move")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{destination}, record.Executions[0].Targets)
	require.Equal(t, "finished", record.Attempts[len(record.Attempts)-1].State)
	restarted := testGuard(guard.ledger.dataDir, nil)
	remaining := Classify("rm a", dir)
	require.True(t, restarted.Check(t.Context(), "move", "retry", remaining).Allowed)
	require.NoError(t, restarted.BeginExecution("move", remaining, "retry"))
	require.False(t, restarted.Check(t.Context(), "move", "repeat", all).Allowed)
	require.Len(t, record.Confirmations, 2)
	require.Equal(t, 4, confirmer.calls)
}

func TestMoveRecoveryReplacesSymlinkEntryWithoutChangingReferent(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("replacement"), 0o600))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(dir, "b"), link))
	c := Classify("mv -T source link", dir)
	require.True(t, guard.Check(t.Context(), "move-link", "one", c).Allowed)
	require.NoError(t, guard.BeginExecution("move-link", c, "one"))
	require.NoError(t, os.Rename(link, filepath.Join(dir, "quarantined-link")))
	require.NoError(t, os.Rename(filepath.Join(dir, "source"), link))
	require.NoError(t, guard.RecordOutcome("move-link", c, "one", c.Targets, false))
	record, err := guard.Ledger().Read("move-link")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	content, err := os.ReadFile(filepath.Join(dir, "b"))
	require.NoError(t, err)
	require.Equal(t, "b", string(content))
}

func TestMoveRecoveryKnownFailureReconcilesProvenMoveAfterRestart(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, []byte("replacement"), 0o600))
	all := Classify("mv source b; rm a", dir)
	require.True(t, guard.Check(t.Context(), "partial-move", "call", all).Allowed)
	require.NoError(t, guard.BeginExecution("partial-move", all, "call"))
	require.NoError(t, os.Rename(destination, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, os.Rename(source, destination))
	guard = testGuard(guard.ledger.dataDir, nil)
	// The complete shell exit is nonzero: no command is globally asserted successful.
	require.NoError(t, guard.RecordOutcome("partial-move", all, "call", nil, false))
	guard = testGuard(guard.ledger.dataDir, nil)
	require.True(t, guard.Check(t.Context(), "partial-move", "retry", Classify("rm a", dir)).Allowed)
	record, err := guard.Ledger().Read("partial-move")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{destination}, record.Executions[0].Targets)
	require.Len(t, record.Confirmations, 2)
}

func TestMoveRecoveryRejectsUnprovenReplacement(t *testing.T) {
	for _, mode := range []string{"unrelated-destination", "source-not-moved", "missing-destination", "pending"} {
		t.Run(mode, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "b")
			require.NoError(t, os.WriteFile(source, []byte("replacement"), 0o600))
			c := Classify("mv source b", dir)
			require.True(t, guard.Check(t.Context(), "unproven-move", "one", c).Allowed)
			require.NoError(t, guard.BeginExecution("unproven-move", c, "one"))
			require.NoError(t, os.Rename(destination, filepath.Join(dir, "quarantined-b")))
			if mode == "unrelated-destination" || mode == "missing-destination" {
				require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
			}
			if mode == "unrelated-destination" || mode == "source-not-moved" {
				require.NoError(t, os.WriteFile(destination, []byte("unrelated bytes"), 0o600))
			}
			if mode == "pending" {
				require.NoError(t, os.Rename(source, destination))
			}
			successes := c.Targets
			if mode == "pending" {
				successes = nil
			}
			require.NoError(t, guard.RecordOutcome("unproven-move", c, "one", successes, mode == "pending"))
			record, err := guard.Ledger().Read("unproven-move")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
			require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
			require.False(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "unproven-move", "retry", c).Allowed)
		})
	}
}

func TestMoveRecoveryRejectsChangedSourceBeforeLaunch(t *testing.T) {
	for _, phase := range []string{"check", "begin"} {
		t.Run(phase, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			source := filepath.Join(dir, "source")
			require.NoError(t, os.WriteFile(source, []byte("replacement"), 0o600))
			c := Classify("mv source b", dir)
			require.True(t, guard.Check(t.Context(), "changed-source", "one", c).Allowed)
			require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
			require.NoError(t, os.WriteFile(source, []byte("replacement"), 0o600))
			if phase == "check" {
				require.False(t, guard.Check(t.Context(), "changed-source", "two", c).Allowed)
			}
			require.Error(t, guard.BeginExecution("changed-source", c, "one"))
			record, err := guard.Ledger().Read("changed-source")
			require.NoError(t, err)
			require.Empty(t, record.Attempts)
			require.Len(t, record.Confirmations, 2)
		})
	}
}

func TestMoveRecoveryProvesCopyAndUnlinkAcrossFilesystems(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	payload := []byte("expected incoming payload")
	require.NoError(t, os.WriteFile(source, payload, 0o600))
	c := Classify("mv source b; rm a", dir)
	require.True(t, guard.Check(t.Context(), "copy-move", "one", c).Allowed)
	require.NoError(t, guard.BeginExecution("copy-move", c, "one"))
	// Simulate the copy/unlink protocol without deleting either original entry.
	require.NoError(t, os.Rename(destination, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, os.WriteFile(destination, payload, 0o600))
	original, err := os.Stat(source)
	require.NoError(t, err)
	replacement, err := os.Stat(destination)
	require.NoError(t, err)
	require.False(t, os.SameFile(original, replacement))
	require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
	guard = testGuard(guard.ledger.dataDir, nil)
	require.NoError(t, guard.RecordOutcome("copy-move", c, "one", []string{destination}, false))
	require.True(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy-move", "retry", Classify("rm a", dir)).Allowed)
	record, err := guard.Ledger().Read("copy-move")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Len(t, record.Confirmations, 2)
}
