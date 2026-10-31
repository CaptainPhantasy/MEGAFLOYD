package deletionguard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCopyRecoveryPreservesUnchangedRemovalRemainder(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source, destination := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, []byte("approved replacement"), 0o600))
	all := ClassifyAll([]string{"cp source b", "rm a"}, dir)
	require.True(t, guard.Check(t.Context(), "copy", "launch", all).Allowed)
	require.NoError(t, guard.BeginExecution("copy", all, "launch"))
	// Preserve the overwritten fixture; no user data is removed.
	require.NoError(t, os.Rename(destination, filepath.Join(dir, "quarantined-b")))
	bytes, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(destination, bytes, 0o600))
	require.NoError(t, guard.RecordOutcome("copy", all, "launch", []string{destination}, false))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Equal(t, "finished", record.Attempts[len(record.Attempts)-1].State)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{destination}, record.Executions[0].Targets)
	require.Len(t, record.Confirmations, 2)
	restarted := testGuard(guard.ledger.dataDir, nil)
	require.True(t, restarted.Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
	require.False(t, restarted.Check(t.Context(), "copy", "repeat-copy", Classify("cp source b", dir)).Allowed)
}

func copyRecoveryFixture(t *testing.T, command string) (*Guard, string, Classification) {
	t.Helper()
	guard, dir, _, _ := recoveryFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("approved replacement"), 0o600))
	all := Classify(command, dir)
	require.True(t, guard.Check(t.Context(), "copy", "launch", all).Allowed)
	require.NoError(t, guard.BeginExecution("copy", all, "launch"))
	return guard, dir, all
}

func simulateFixtureCopy(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-b")))
	bytes, err := os.ReadFile(filepath.Join(dir, "source"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), bytes, 0o600))
}

func TestCopyRecoveryKnownFailureReconcilesAfterRestart(t *testing.T) {
	guard, dir, all := copyRecoveryFixture(t, "cp source b; rm a")
	simulateFixtureCopy(t, dir)
	guard = testGuard(guard.ledger.dataDir, nil)
	require.NoError(t, guard.RecordOutcome("copy", all, "launch", nil, false))
	guard = testGuard(guard.ledger.dataDir, nil)
	require.True(t, guard.Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{filepath.Join(dir, "b")}, record.Executions[0].Targets)
	require.Len(t, record.Confirmations, 2)
	content, err := os.ReadFile(filepath.Join(dir, "source"))
	require.NoError(t, err)
	require.Equal(t, "approved replacement", string(content))
}

func TestCopyRecoveryBackgroundPartialFailure(t *testing.T) {
	guard, dir, all := copyRecoveryFixture(t, "cp source b; rm a")
	job, err := guard.BindBackground("copy", all, "launch", BackgroundJob{SessionID: "copy", ToolCallID: "launch", ShellID: "001", ExecutionID: "copy-background", Command: "cp source b; rm a", WorkingDir: dir, WorkspaceDir: dir, Classification: all})
	require.NoError(t, err)
	simulateFixtureCopy(t, dir)
	require.NoError(t, guard.ReconcileBackground("copy", BackgroundResult{Job: job, Done: true, ExitCode: 7, Error: "independent removal failed"}))
	require.True(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Len(t, record.Confirmations, 2)
	require.Equal(t, "finished", record.Attempts[1].State)
	require.Equal(t, "independent removal failed", record.Attempts[1].BackgroundOutcome.Error)
}

func TestCopyRecoveryRejectsUnprovenReplacement(t *testing.T) {
	for _, mode := range []string{"unrelated bytes", "source replaced", "source removed", "destination symlink", "destination missing", "pending"} {
		t.Run(mode, func(t *testing.T) {
			guard, dir, all := copyRecoveryFixture(t, "cp source b; rm a")
			simulateFixtureCopy(t, dir)
			switch mode {
			case "unrelated bytes":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("unapproved contents"), 0o600))
			case "source replaced":
				require.NoError(t, os.Rename(filepath.Join(dir, "source"), filepath.Join(dir, "quarantined-source")))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("approved replacement"), 0o600))
			case "source removed":
				require.NoError(t, os.Rename(filepath.Join(dir, "source"), filepath.Join(dir, "quarantined-source")))
			case "destination symlink":
				require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-result")))
				require.NoError(t, os.Symlink(filepath.Join(dir, "source"), filepath.Join(dir, "b")))
			case "destination missing":
				require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-result")))
			}
			successes := []string{filepath.Join(dir, "b")}
			if mode == "pending" {
				successes = nil
			}
			require.NoError(t, guard.RecordOutcome("copy", all, "launch", successes, mode == "pending"))
			record, err := guard.Ledger().Read("copy")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
			require.Len(t, record.Confirmations, 2)
			require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
			require.False(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
		})
	}
}

func TestCopyRecoveryRejectsChangedSourceAndOptionsBeforeLaunch(t *testing.T) {
	for _, mode := range []string{"content", "substitution", "source symlink", "options"} {
		t.Run(mode, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			source := filepath.Join(dir, "source")
			require.NoError(t, os.WriteFile(source, []byte("approved replacement"), 0o600))
			c := Classify("cp source b", dir)
			require.True(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
			switch mode {
			case "content":
				require.NoError(t, os.WriteFile(source, []byte("changed"), 0o600))
			case "substitution":
				require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
				require.NoError(t, os.WriteFile(source, []byte("approved replacement"), 0o600))
			case "source symlink":
				require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
				require.NoError(t, os.Symlink(filepath.Join(dir, "quarantined-source"), source))
			case "options":
				c = Classify("cp -p source b", dir)
			}
			require.False(t, guard.Check(t.Context(), "copy", "retry", c).Allowed)
			require.Error(t, guard.BeginExecution("copy", c, "launch"))
			record, err := guard.Ledger().Read("copy")
			require.NoError(t, err)
			require.Empty(t, record.Attempts)
			require.Len(t, record.Confirmations, 2)
		})
	}
}

func TestCopyRecoveryRefusesNonregularOrSymlinkSourceBeforeConfirmation(t *testing.T) {
	for _, mode := range []string{"symlink", "directory", "device"} {
		t.Run(mode, func(t *testing.T) {
			guard, dir, _, confirmer := recoveryFixture(t)
			source := filepath.Join(dir, "source")
			switch mode {
			case "symlink":
				require.NoError(t, os.Symlink(filepath.Join(dir, "a"), source))
			case "directory":
				require.NoError(t, os.Mkdir(source, 0o700))
			case "device":
				source = os.DevNull
			}
			before := confirmer.calls
			decision := guard.Check(t.Context(), "copy", "approve", Classify("cp "+source+" b", dir))
			require.False(t, decision.Allowed, decision.Reason)
			require.Equal(t, before, confirmer.calls)
			record, err := guard.Ledger().Read("copy")
			require.NoError(t, err)
			require.Empty(t, record.Confirmations)
		})
	}
}

func TestCopyRecoveryPlainCopyKeepsDestinationMode(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, []byte("payload"), 0o600))
	require.NoError(t, os.Chmod(target, 0o644))
	c := Classify("cp source b", dir)
	require.True(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
	require.NoError(t, guard.BeginExecution("copy", c, "launch"))
	require.NoError(t, os.Rename(target, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, os.WriteFile(target, []byte("payload"), 0o644))
	require.NoError(t, os.Chmod(target, 0o644))
	require.NoError(t, guard.RecordOutcome("copy", c, "launch", c.Targets, false))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, "finished", record.Attempts[1].State)
}

type copyConfirmationMutator struct {
	mutate func(ConfirmationRequest)
}

func (c copyConfirmationMutator) Confirm(_ context.Context, request ConfirmationRequest) (bool, error) {
	c.mutate(request)
	return true, nil
}

func TestCopyRecoveryBindsSourceBeforeBothConfirmations(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	source := filepath.Join(dir, "source")
	require.NoError(t, os.WriteFile(source, []byte("approved replacement"), 0o600))
	guard.confirmer = copyConfirmationMutator{mutate: func(req ConfirmationRequest) {
		if req.Stage == 2 {
			require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-source")))
			require.NoError(t, os.WriteFile(source, []byte("substituted bytes"), 0o600))
		}
	}}
	c := Classify("cp source b", dir)
	require.False(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
	require.Error(t, guard.BeginExecution("copy", c, "launch"))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Len(t, record.Confirmations, 2)
	require.Empty(t, record.Attempts)
	require.NotEmpty(t, record.Approvals[0].Copies[filepath.Join(dir, "b")].Identity)
}

func TestCopyRecoveryRejectsRetargetedDestinationParent(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("approved replacement"), 0o600))
	for _, name := range []string{"original-parent", "other-parent"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "destination"), []byte("old"), 0o600))
	}
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Symlink(filepath.Join(dir, "original-parent"), alias))
	c := Classify("cp source alias/destination; rm a", dir)
	require.True(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
	require.NoError(t, guard.BeginExecution("copy", c, "launch"))
	require.NoError(t, os.Rename(alias, filepath.Join(dir, "quarantined-alias")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "other-parent"), alias))
	require.NoError(t, os.WriteFile(filepath.Join(alias, "destination"), []byte("approved replacement"), 0o600))
	require.NoError(t, guard.RecordOutcome("copy", c, "launch", []string{filepath.Join(alias, "destination")}, false))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[1].State)
	require.False(t, guard.Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
}

func TestCopyRecoveryRejectsChangedParentBeforeLaunch(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	for _, name := range []string{"first-source-parent", "second-source-parent"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "source"), []byte("same bytes"), 0o600))
	}
	alias := filepath.Join(dir, "source-alias")
	require.NoError(t, os.Symlink(filepath.Join(dir, "first-source-parent"), alias))
	c := Classify("cp source-alias/source b", dir)
	require.True(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
	require.NoError(t, os.Rename(alias, filepath.Join(dir, "quarantined-source-alias")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "second-source-parent"), alias))
	require.False(t, guard.Check(t.Context(), "copy", "check-changed-parent", c).Allowed)
	require.Error(t, guard.BeginExecution("copy", c, "launch"))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Len(t, record.Confirmations, 2)
}

func TestCopyRecoveryPreserveOptionsStopAtDelimiterAndSkipDirectoryValue(t *testing.T) {
	for _, signature := range []string{
		`[["cp","--","/fixture/-p","/fixture/destination"]]`,
		`[["cp","-t","-p","/fixture/source","/fixture/-p/source"]]`,
		`[["cp","--target-directory=-p","/fixture/source","/fixture/-p/source"]]`,
	} {
		require.False(t, copyPreservesMetadata(signature), signature)
	}
	require.True(t, copyPreservesMetadata(`[["cp","-pv","--","/fixture/source","/fixture/destination"]]`))
}

func TestCopyRecoveryFailedUnchangedCopyIsNotCompleted(t *testing.T) {
	for _, explicitSuccess := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed or skipped", true: "explicit successful noop"}[explicitSuccess], func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			payload := []byte("identical existing contents")
			source, target := filepath.Join(dir, "source"), filepath.Join(dir, "b")
			require.NoError(t, os.WriteFile(source, payload, 0o600))
			require.NoError(t, os.WriteFile(target, payload, 0o600))
			all := ClassifyAll([]string{"cp source b", "rm a"}, dir)
			require.True(t, guard.Check(t.Context(), "copy", "approve", all).Allowed)
			require.NoError(t, guard.BeginExecution("copy", all, "launch"))
			// No file changes: a failed/skipped copy must not inherit success merely
			// because the destination happened to satisfy its desired postcondition.
			var successful []string
			if explicitSuccess {
				successful = []string{target}
			}
			require.NoError(t, guard.RecordOutcome("copy", all, "launch", successful, false))
			record, err := guard.Ledger().Read("copy")
			require.NoError(t, err)
			require.Len(t, record.Confirmations, 2)
			require.Equal(t, "finished", record.Attempts[1].State)
			if explicitSuccess {
				require.Len(t, record.Executions, 1)
				require.Equal(t, []string{target}, record.Executions[0].Targets)
				require.True(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
			} else {
				require.Empty(t, record.Executions)
				restarted := testGuard(guard.ledger.dataDir, nil)
				require.True(t, restarted.Check(t.Context(), "copy", "retry-original-remainder", all).Allowed)
				require.False(t, restarted.Check(t.Context(), "copy", "unproven-shrink", Classify("rm a", dir)).Allowed)
			}
		})
	}
}

func TestCopyRecoveryFailedMatchingCopyTimestampChangeIsNotCompleted(t *testing.T) {
	guard, dir, _, _ := recoveryFixture(t)
	payload := []byte("identical existing contents")
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(source, payload, 0o600))
	require.NoError(t, os.WriteFile(target, payload, 0o600))
	c := ClassifyAll([]string{"cp source b", "rm a"}, dir)
	require.True(t, guard.Check(t.Context(), "copy", "approve", c).Allowed)
	require.NoError(t, guard.BeginExecution("copy", c, "launch"))
	// An unrelated metadata update cannot prove a failed/skipped data copy.
	changed := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(target, changed, changed))
	require.NoError(t, guard.RecordOutcome("copy", c, "launch", nil, false))
	record, err := guard.Ledger().Read("copy")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[1].State)
	require.False(t, testGuard(guard.ledger.dataDir, nil).Check(t.Context(), "copy", "retry", Classify("rm a", dir)).Allowed)
	require.Len(t, record.Confirmations, 2)
}

func TestCopyRecoveryMissingOriginalDestinationPayloadFailsClosed(t *testing.T) {
	for _, field := range []string{"approval", "launch"} {
		t.Run(field, func(t *testing.T) {
			guard, dir, c := copyRecoveryFixture(t, "cp source b")
			target := filepath.Join(dir, "b")
			simulateFixtureCopy(t, dir)
			record, err := guard.Ledger().Read("copy")
			require.NoError(t, err)
			if field == "approval" {
				bound := record.Approvals[0].Copies[target]
				require.NotEmpty(t, bound.OriginalDestinationContent)
				bound.OriginalDestinationContent = ""
				record.Approvals[0].Copies[target] = bound
			} else {
				bound := record.Attempts[0].Copies[target]
				require.NotEmpty(t, bound.OriginalDestinationContent)
				bound.OriginalDestinationContent = ""
				record.Attempts[0].Copies[target] = bound
			}
			require.NoError(t, guard.ledger.write("copy", record))
			err = guard.RecordOutcome("copy", c, "launch", c.Targets, false)
			if field == "approval" {
				require.ErrorIs(t, err, ErrNotConfirmed)
			} else {
				require.NoError(t, err)
			}
			record, err = guard.Ledger().Read("copy")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
		})
	}
}

func TestCopyRecoveryRefusesCompoundSourceOperationsBeforeConfirmations(t *testing.T) {
	for _, command := range []string{"truncate -s 0 b; cp source b", "rm b; cp source b"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, confirmer := recoveryFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("approved source"), 0o600))
			c := Classify(command, dir)
			require.True(t, c.IsDestructive())
			require.Empty(t, c.Unresolved, "regression must reach recovery validation")
			before := confirmer.calls
			decision := guard.Check(t.Context(), "compound-copy", "approve", c)
			require.False(t, decision.Allowed, "compound copy must not be treated as source-free")
			require.Equal(t, before, confirmer.calls)
			record, err := guard.Ledger().Read("compound-copy")
			require.NoError(t, err)
			require.Empty(t, record.Confirmations)
			require.Empty(t, record.Approvals)
		})
	}
}

func TestCopyRecoveryLegacyCompoundApprovalCannotLaunchChangedSource(t *testing.T) {
	for _, command := range []string{"truncate -s 0 b; cp source b", "rm b; cp source b"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			source := filepath.Join(dir, "source")
			require.NoError(t, os.WriteFile(source, []byte("original source"), 0o600))
			c := Classify(command, dir)
			snapshots, err := snapshotTargets(c.Targets)
			require.NoError(t, err)
			record, err := guard.Ledger().Read("legacy-compound")
			require.NoError(t, err)
			// This is the protected record produced by the previous implementation:
			// two answers and target snapshots, with compound copy source unbound.
			record.Approvals = []Approval{{TargetSet: c.Digest(), Targets: c.Targets, Command: c.Command, Recursive: c.Recursive, Snapshots: snapshots, Operations: c.Operations}}
			for stage := 1; stage <= 2; stage++ {
				record.Confirmations = append(record.Confirmations, Confirmation{ID: fmt.Sprintf("legacy-%d", stage), Stage: stage, TargetSet: c.Digest(), Targets: c.Targets, Source: humanSource, RecordedAt: time.Unix(1700000000+int64(stage), 0).UTC().Format(time.RFC3339Nano)})
			}
			require.NoError(t, guard.ledger.write("legacy-compound", record))
			require.NoError(t, os.Rename(source, filepath.Join(dir, "quarantined-original-source")))
			require.NoError(t, os.WriteFile(source, []byte("substituted source"), 0o600))
			restarted := testGuard(guard.ledger.dataDir, nil)
			require.Error(t, restarted.BeginExecution("legacy-compound", c, "launch"), "changed compound source cannot bypass locked launch validation")
			require.False(t, restarted.Check(t.Context(), "legacy-compound", "retry", c).Allowed)
			after, err := guard.Ledger().Read("legacy-compound")
			require.NoError(t, err)
			require.Empty(t, after.Attempts)
			require.Len(t, after.Confirmations, 2)
		})
	}
}

func TestCopyRecoveryCompoundMoveAndCopyDescriptorsCannotSkipBinding(t *testing.T) {
	dir := t.TempDir()
	target, source := filepath.Join(dir, "destination"), filepath.Join(dir, "source")
	for _, command := range []string{"cp", "mv"} {
		for _, first := range []bool{false, true} {
			operations := [][]string{{"truncate", "-s", "0"}, {command, source, target}}
			if first {
				operations[0], operations[1] = operations[1], operations[0]
			}
			encoded, err := json.Marshal(operations)
			require.NoError(t, err)
			signature := string(encoded)
			require.True(t, compoundSourceOperation(signature))
			c := Classification{Effect: EffectPermanentDeletion, Targets: []string{target}, Operations: map[string]string{target: signature}}
			approval := Approval{TargetSet: c.Digest(), Targets: c.Targets, Snapshots: map[string]string{target: "snapshot"}, Operations: c.Operations}
			require.False(t, sameOperation(approval, c))
			require.False(t, unchangedCopySource(approval, target))
			require.False(t, unchangedMoveSource(approval, target))
			_, err = captureCopySources(c)
			require.Error(t, err)
			_, err = captureMoveSources(c)
			require.Error(t, err)
		}
	}
	require.False(t, compoundSourceOperation(`[["truncate","-s","0"],["rm"]]`), "compound operations without external source binding retain existing handling")
}

func TestCopyRecoveryAttributePreservationIsRefusedBeforeConfirmation(t *testing.T) {
	for _, option := range []string{"-a", "-va", "-pa", "-p", "-vp"} {
		t.Run(option, func(t *testing.T) {
			guard, dir, _, confirmer := recoveryFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "source"), []byte("payload"), 0o600))
			c := Classify("cp "+option+" source b; rm source", dir)
			require.True(t, c.IsDestructive())
			require.Empty(t, c.Unresolved)
			before := confirmer.calls
			decision := guard.Check(t.Context(), "archive-copy", "approve", c)
			require.False(t, decision.Allowed, "POSIX metadata alone must not authorize archive-copy completion")
			require.Equal(t, before, confirmer.calls)
			record, err := guard.Ledger().Read("archive-copy")
			require.NoError(t, err)
			require.Empty(t, record.Confirmations)
			require.Empty(t, record.Approvals)
		})
	}
}

func TestCopyRecoveryArchiveOptionsCannotHideInOrBorrowOperands(t *testing.T) {
	for _, signature := range []string{
		`[["cp","-p","/fixture/source","/fixture/destination"]]`,
		`[["cp","--preserve=mode","/fixture/source","/fixture/destination"]]`,
		`[["cp","-a","/fixture/source","/fixture/destination"]]`,
		`[["cp","-vpa","/fixture/source","/fixture/destination"]]`,
		`[["cp","-atdestination","/fixture/source","/fixture/destination"]]`,
		`[["cp","--archive","/fixture/source","/fixture/destination"]]`,
		`[["cp","--archive","source"]]`,
	} {
		require.True(t, unsupportedCopyAttributes(signature), signature)
	}
	for _, signature := range []string{
		`[["cp","-t","-a","/fixture/source","/fixture/-a/source"]]`,
		`[["cp","-t","-p","/fixture/source","/fixture/-p/source"]]`,
		`[["cp","-tp","/fixture/source","/fixture/p/source"]]`,
		`[["cp","--","--preserve","destination"]]`,
		`[["cp","--","-p","destination"]]`,
		`[["cp","-ta","/fixture/source","/fixture/a/source"]]`,
		`[["cp","--target-directory","--archive","/fixture/source","/fixture/--archive/source"]]`,
		`[["cp","--target-directory=-a","/fixture/source","/fixture/-a/source"]]`,
		`[["cp","--","-a","destination"]]`,
	} {
		require.False(t, unsupportedCopyAttributes(signature), signature)
	}
}

func TestCopyRecoveryLegacyArchiveCannotAuthorizeSourceRemoval(t *testing.T) {
	for _, option := range []string{"-a", "--archive", "-va", "-p", "--preserve", "--preserve=mode,ownership,timestamps"} {
		t.Run(option, func(t *testing.T) {
			guard, dir, _, _ := recoveryFixture(t)
			source, target := filepath.Join(dir, "source"), filepath.Join(dir, "b")
			require.NoError(t, os.WriteFile(source, []byte("archive payload"), 0o600))
			plain := Classify("cp source b", dir)
			copies, err := captureCopySources(plain)
			require.NoError(t, err)
			info, err := os.Stat(source)
			require.NoError(t, err)
			bound := copies[target]
			bound.PreserveMetadata, err = copyMetadata(info)
			require.NoError(t, err)
			// Reconstruct the old archive descriptor directly so the long-option
			// regression also covers protected history from older classifiers.
			encoded, err := json.Marshal([][]string{{"cp", option, source, target}})
			require.NoError(t, err)
			c := ClassifyAll([]string{"cp -a source b", "rm source"}, dir)
			c.Operations[target] = string(encoded)
			snapshots, err := snapshotTargets(c.Targets)
			require.NoError(t, err)
			record, err := guard.Ledger().Read("legacy-archive")
			require.NoError(t, err)
			record.Approvals = []Approval{{TargetSet: c.Digest(), Targets: c.Targets, Command: c.Command, Recursive: c.Recursive, Snapshots: snapshots, Operations: c.Operations, Copies: map[string]CopySource{target: bound}}}
			for stage := 1; stage <= 2; stage++ {
				record.Confirmations = append(record.Confirmations, Confirmation{ID: fmt.Sprintf("archive-%d", stage), Stage: stage, TargetSet: c.Digest(), Targets: c.Targets, Source: humanSource, RecordedAt: time.Unix(1700000000+int64(stage), 0).UTC().Format(time.RFC3339Nano)})
			}
			require.NoError(t, guard.ledger.write("legacy-archive", record))
			require.Error(t, guard.BeginExecution("legacy-archive", c, "launch"))
			require.False(t, guard.Check(t.Context(), "legacy-archive", "retry", c).Allowed)
			require.False(t, guard.Check(t.Context(), "legacy-archive", "source-remainder", Classify("rm source", dir)).Allowed)
			// Even matching bytes and all metadata currently checked cannot prove the
			// archive's unverified extended attributes. Preserve the old fixture bytes.
			simulateFixtureCopy(t, dir)
			require.NoError(t, os.Chtimes(target, info.ModTime(), info.ModTime()))
			proven, err := copyPostcondition(target, string(encoded), bound)
			require.NoError(t, err)
			require.False(t, proven)
			record, err = guard.Ledger().Read("legacy-archive")
			require.NoError(t, err)
			require.Empty(t, record.Executions)
			require.Empty(t, record.Attempts)
			require.Len(t, record.Confirmations, 2)
		})
	}
}

func TestCopyRecoveryActualACLChangeCannotAuthorizePreservingCopy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("isolated native ACL fixture uses macOS chmod; preservation rejection runs on every platform")
	}
	guard, dir, _, confirmer := recoveryFixture(t)
	source := filepath.Join(dir, "source")
	require.NoError(t, os.WriteFile(source, []byte("private ACL regression fixture"), 0o600))
	before, err := snapshot(source)
	require.NoError(t, err)
	info, err := os.Lstat(source)
	require.NoError(t, err)
	ordinaryBefore, err := copyMetadata(info)
	require.NoError(t, err)
	// Modify only this test-owned file inside its private temporary directory.
	// No account, service, workspace, or existing user ACL is changed.
	output, err := exec.CommandContext(t.Context(), "/bin/chmod", "+a", "everyone allow read", source).CombinedOutput()
	require.NoError(t, err, string(output))
	output, err = exec.CommandContext(t.Context(), "/bin/ls", "-lde", source).CombinedOutput()
	require.NoError(t, err, string(output))
	require.Regexp(t, `(?m)^\s*0: (?:everyone|group:everyone|ABCDEFAB-CDEF-ABCD-EFAB-CDEF0000000C) allow read$`, string(output))
	after, err := snapshot(source)
	require.NoError(t, err)
	info, err = os.Lstat(source)
	require.NoError(t, err)
	ordinaryAfter, err := copyMetadata(info)
	require.NoError(t, err)
	require.Equal(t, before, after, "the ordinary source fingerprint does not capture this real ACL change")
	require.Equal(t, ordinaryBefore, ordinaryAfter, "mode/mtime/uid/gid do not prove requested ACL preservation")
	calls := confirmer.calls
	decision := guard.Check(t.Context(), "acl-copy", "approve", Classify("cp -p source b; rm source", dir))
	require.False(t, decision.Allowed)
	require.Equal(t, calls, confirmer.calls)
	record, err := guard.Ledger().Read("acl-copy")
	require.NoError(t, err)
	require.Empty(t, record.Confirmations)
}
