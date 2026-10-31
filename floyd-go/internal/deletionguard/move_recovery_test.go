package deletionguard

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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

func TestMoveRecoveryRejectsRecreatedDestinationAfterCopyAndUnlink(t *testing.T) {
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
	require.False(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy-move", "retry", Classify("rm a", dir)).Allowed)
	record, err := guard.Ledger().Read("copy-move")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
	require.Len(t, record.Confirmations, 2)
}

type moveDeviceFixtureInfo struct {
	os.FileInfo
	device uint64
	inode  uint64
}

func (info moveDeviceFixtureInfo) Sys() any {
	return struct{ Dev, Ino uint64 }{info.device, info.inode}
}

func TestMoveRecoveryDeviceMismatchCannotBindSource(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "source")
	require.NoError(t, os.WriteFile(file, []byte("preserved source"), 0o600))
	source, err := os.Lstat(file)
	require.NoError(t, err)
	parent, err := os.Stat(dir)
	require.NoError(t, err)
	_, err = verifyMoveDevices(moveDeviceFixtureInfo{FileInfo: source, device: 1, inode: 100}, moveDeviceFixtureInfo{FileInfo: parent, device: 2, inode: 200})
	require.ErrorContains(t, err, "cross-filesystem")
	entry, err := verifyMoveDevices(moveDeviceFixtureInfo{FileInfo: source, device: 1, inode: 100}, moveDeviceFixtureInfo{FileInfo: parent, device: 1, inode: 200})
	require.NoError(t, err)
	require.Equal(t, uint64(100), entry.Inode)
}

func TestMoveRecoveryActualLinuxCrossFilesystemDeniedBeforeConfirmation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("actual separate-device fixture uses Linux /dev/shm; device validation runs on all platforms")
	}
	destinationDir, err := os.MkdirTemp("/dev/shm", "megafloyd-move-device-")
	if err != nil {
		t.Skipf("private /dev/shm fixture unavailable: %v", err)
	}
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(destinationDir)) })
	guard, dir, _, confirmer := recoveryFixture(t)
	source, target := filepath.Join(dir, "source"), filepath.Join(destinationDir, "destination")
	require.NoError(t, os.WriteFile(source, []byte("source must remain"), 0o600))
	require.NoError(t, os.WriteFile(target, []byte("target must remain"), 0o600))
	sourceInfo, err := os.Lstat(source)
	require.NoError(t, err)
	parentInfo, err := os.Stat(destinationDir)
	require.NoError(t, err)
	sourceEntry, err := moveEntryIdentity(sourceInfo)
	require.NoError(t, err)
	parentEntry, err := moveEntryIdentity(parentInfo)
	require.NoError(t, err)
	if sourceEntry.Device == parentEntry.Device {
		t.Skip("temporary source and /dev/shm share a device; no actual cross-filesystem fixture")
	}
	c := Classify(fmt.Sprintf("mv %q %q", source, target), dir)
	require.True(t, c.IsDestructive())
	require.Empty(t, c.Unresolved)
	calls := confirmer.calls
	decision := guard.Check(t.Context(), "cross-filesystem", "approve", c)
	require.False(t, decision.Allowed)
	require.Contains(t, decision.Reason, "cross-filesystem")
	require.Equal(t, calls, confirmer.calls)
	require.Error(t, guard.BeginExecution("cross-filesystem", c, "launch"))
	record, err := guard.Ledger().Read("cross-filesystem")
	require.NoError(t, err)
	require.Empty(t, record.Confirmations)
	require.Empty(t, record.Attempts)
	content, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "source must remain", string(content))
	content, err = os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "target must remain", string(content))
}

func TestMoveRecoveryLegacyEntryProofCannotBeReusedAfterRestart(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source := filepath.Join(dir, "source")
	require.NoError(t, os.WriteFile(source, []byte("original source"), 0o600))
	c := Classify("mv source b; rm a", dir)
	require.True(t, guard.Check(t.Context(), "legacy-move", "approve", c).Allowed)
	record, err := guard.Ledger().Read("legacy-move")
	require.NoError(t, err)
	target := filepath.Join(dir, "b")
	bound := record.Approvals[0].Moves[target]
	require.NotNil(t, bound.Entry)
	bound.Entry = nil
	record.Approvals[0].Moves[target] = bound
	require.NoError(t, guard.ledger.write("legacy-move", record))
	restarted := testGuard(guard.ledger.dataDir, nil)
	require.False(t, restarted.Check(t.Context(), "legacy-move", "retry", c).Allowed)
	require.False(t, restarted.Check(t.Context(), "legacy-move", "subset", Classify("rm a", dir)).Allowed)
	require.Error(t, restarted.BeginExecution("legacy-move", c, "launch"))
	record, err = restarted.Ledger().Read("legacy-move")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Len(t, record.Confirmations, 2)
}

func TestMoveRecoveryMissingLaunchEntryProofCannotComplete(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, []byte("original entry"), 0o600))
	c := Classify("mv source b; rm a", dir)
	require.True(t, guard.Check(t.Context(), "move", "approve", c).Allowed)
	require.NoError(t, guard.BeginExecution("move", c, "launch"))
	record, err := guard.Ledger().Read("move")
	require.NoError(t, err)
	bound := record.Attempts[0].Moves[target]
	require.NotNil(t, bound.Entry)
	bound.Entry = nil
	record.Attempts[0].Moves[target] = bound
	require.NoError(t, guard.ledger.write("move", record))
	require.NoError(t, os.Rename(target, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, os.Rename(source, target))
	require.NoError(t, guard.RecordOutcome("move", c, "launch", []string{target}, false))
	record, err = guard.Ledger().Read("move")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[1].State)
}

func TestMoveRecoverySameDeviceDifferentMountCannotRename(t *testing.T) {
	entry := MoveEntryIdentity{Device: 1, Inode: 100, Type: 0}
	_, err := verifyMoveMounts(entry, "mount-original", "mount-bind-view")
	require.ErrorContains(t, err, "cross-mount")
	_, err = verifyMoveMounts(entry, "", "mount-original")
	require.ErrorContains(t, err, "unavailable")
	result, err := verifyMoveMounts(entry, "mount-original", "mount-original")
	require.NoError(t, err)
	require.Equal(t, "mount-original", result.MountID)
}

func TestMoveRecoveryLegacyCompletedSubsetWithoutEntryProofCannotUnlockRemainder(t *testing.T) {
	for _, missing := range []string{"entry", "mount", "inode"} {
		t.Run(missing, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("original entry"), 0o600))
			c := Classify("mv source b; rm a", dir)
			require.True(t, guard.Check(t.Context(), "legacy-partial", "approve", c).Allowed)
			record, err := guard.Ledger().Read("legacy-partial")
			require.NoError(t, err)
			target := filepath.Join(dir, "b")
			bound := record.Approvals[0].Moves[target]
			require.NotNil(t, bound.Entry)
			switch missing {
			case "entry":
				bound.Entry = nil
			case "mount":
				bound.Entry.MountID = ""
			default:
				bound.Entry.Inode = 0
			}
			record.Approvals[0].Moves[target] = bound
			record.Executions = []Execution{{ID: "legacy-degraded-copy", TargetSet: Digest([]string{target}), ApprovalSet: c.Digest(), Targets: []string{target}, ToolCallID: "old-call", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
			require.NoError(t, guard.ledger.write("legacy-partial", record))
			restarted := testGuard(guard.ledger.dataDir, nil)
			remaining := Classify("rm a", dir)
			require.False(t, restarted.Check(t.Context(), "legacy-partial", "retry", remaining).Allowed)
			require.Error(t, restarted.BeginExecution("legacy-partial", remaining, "retry"))
		})
	}
}
