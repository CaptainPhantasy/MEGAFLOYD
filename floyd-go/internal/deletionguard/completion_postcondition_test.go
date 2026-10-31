package deletionguard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCompletionSkippedTruncateIgnoresUnrelatedMetadataChange(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	classification := Classify("touch b; if false; then truncate -s 0 b; fi", dir)
	require.True(t, guard.Check(t.Context(), "skip-metadata", "call", classification).Allowed)
	require.NoError(t, guard.BeginExecution("skip-metadata", classification, "call"))
	target := filepath.Join(dir, "b")
	info, err := os.Stat(target)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(target, info.ModTime(), info.ModTime().Add(time.Second)))
	require.NoError(t, guard.RecordOutcome("skip-metadata", classification, "call", classification.Targets, false))
	record, err := guard.Ledger().Read("skip-metadata")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "b", string(content))
}

func TestCompletionProvesExactTruncateSize(t *testing.T) {
	for _, option := range []string{"-s 0", "-s0", "--size=0", "--size 0"} {
		t.Run(option, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			classification := Classify("truncate "+option+" b", dir)
			require.True(t, guard.Check(t.Context(), "truncate", "call", classification).Allowed)
			require.NoError(t, guard.BeginExecution("truncate", classification, "call"))
			target := filepath.Join(dir, "b")
			// Move original bytes to quarantine, then create the verified final fixture.
			require.NoError(t, os.Rename(target, filepath.Join(dir, "quarantined-original-b")))
			require.NoError(t, os.WriteFile(target, nil, 0o600))
			require.NoError(t, guard.RecordOutcome("truncate", classification, "call", classification.Targets, false))
			record, err := guard.Ledger().Read("truncate")
			require.NoError(t, err)
			require.Len(t, record.Executions, 1)
			require.Equal(t, "finished", record.Attempts[len(record.Attempts)-1].State)
			require.False(t, guard.Check(t.Context(), "truncate", "repeat", classification).Allowed)
		})
	}
}

func TestCompletionUnprovenInPlaceEffectsRemainUncertain(t *testing.T) {
	for _, command := range []string{"shred b", "dd if=/dev/zero of=b count=1", "truncate -s +1 b"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			classification := Classify(command, dir)
			require.True(t, guard.Check(t.Context(), "unproven", "call", classification).Allowed)
			require.NoError(t, guard.BeginExecution("unproven", classification, "call"))
			target := filepath.Join(dir, "b")
			info, err := os.Stat(target)
			require.NoError(t, err)
			require.NoError(t, os.Chtimes(target, info.ModTime(), info.ModTime().Add(time.Second)))
			require.NoError(t, guard.RecordOutcome("unproven", classification, "call", classification.Targets, false))
			record, err := guard.Ledger().Read("unproven")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
			require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
		})
	}
}

func TestCompletionMissingInPlaceTargetDoesNotProveTransformation(t *testing.T) {
	for _, command := range []string{"truncate -s 0 b", "dd if=/dev/zero of=b count=1", "shred b"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			classification := Classify(command, dir)
			require.True(t, guard.Check(t.Context(), "missing", "call", classification).Allowed)
			require.NoError(t, guard.BeginExecution("missing", classification, "call"))
			require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-b")))
			require.NoError(t, guard.RecordOutcome("missing", classification, "call", classification.Targets, false))
			record, err := guard.Ledger().Read("missing")
			require.NoError(t, err)
			require.Empty(t, record.Executions, "quarantine is not proof of the requested in-place effect")
			require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
		})
	}
}

func TestCompletionInPlaceSymlinkCannotApproveReferencedData(t *testing.T) {
	for _, command := range []string{"truncate -s 0 link", "dd if=/dev/zero of=link count=1", "shred link", "> link"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, confirmer := recoveryFixture(t)
			require.NoError(t, os.Symlink(filepath.Join(dir, "b"), filepath.Join(dir, "link")))
			classification := Classify(command, dir)
			before := confirmer.calls
			decision := guard.Check(t.Context(), "symlink", "call", classification)
			require.False(t, decision.Allowed)
			require.Contains(t, decision.Reason, "symbolic link")
			require.Equal(t, before, confirmer.calls)
			require.Error(t, guard.BeginExecution("symlink", classification, "call"))
		})
	}
}

func TestCompletionUnlinkSymlinkKeepsItsOwnIdentity(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(dir, "b"), filepath.Join(dir, "link")))
	classification := Classify("rm link", dir)
	require.True(t, guard.Check(t.Context(), "unlink-link", "call", classification).Allowed)
	// Changing the referent does not change the exact link that rm would unlink.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("referent stays preserved"), 0o600))
	require.NoError(t, guard.BeginExecution("unlink-link", classification, "call"))
	require.NoError(t, os.Rename(filepath.Join(dir, "link"), filepath.Join(dir, "quarantined-link")))
	require.NoError(t, guard.RecordOutcome("unlink-link", classification, "call", classification.Targets, false))
	record, err := guard.Ledger().Read("unlink-link")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	content, err := os.ReadFile(filepath.Join(dir, "b"))
	require.NoError(t, err)
	require.Equal(t, "referent stays preserved", string(content))
}
