package deletionguard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApprovalBindingRejectsConfirmationOnlyReplacement(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	record.Approvals = nil
	require.NoError(t, guard.Ledger().write("s", record))
	require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-original-b")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("replacement"), 0o600))
	require.False(t, guard.Check(t.Context(), "s", "check", all).Allowed, "confirmation-only history cannot approve a replacement")
	require.Error(t, guard.BeginExecution("s", all, "launch"), "launch cannot bind fresh snapshots to old answers")
	record, err = guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Len(t, record.Confirmations, 2)
}

func TestApprovalBindingRevalidatesBeforeLaunch(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	require.True(t, guard.Check(t.Context(), "s", "checked", all).Allowed)
	require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-original-b")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("replacement"), 0o600))
	require.Error(t, guard.BeginExecution("s", all, "changed-before-launch"))
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Empty(t, record.Executions)
	require.Len(t, record.Confirmations, 2)
}

func TestApprovalBindingChecksAfterLaunchPreparation(t *testing.T) {
	guard, dir, all, _ := recoveryFixture(t)
	originalClock := guard.now
	guard.now = func() time.Time {
		require.NoError(t, os.Rename(filepath.Join(dir, "b"), filepath.Join(dir, "quarantined-before-launch")))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("replacement during launch preparation"), 0o600))
		return originalClock()
	}
	require.Error(t, guard.BeginExecution("s", all, "replacement-during-preparation"))
	record, err := guard.Ledger().Read("s")
	require.NoError(t, err)
	require.Empty(t, record.Attempts)
	require.Empty(t, record.Executions)
}

func TestApprovalBindingRequiresCompleteMetadata(t *testing.T) {
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
