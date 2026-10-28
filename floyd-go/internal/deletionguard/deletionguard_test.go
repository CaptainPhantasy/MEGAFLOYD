package deletionguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeletionClassificationByEffect(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "existing.log"), []byte("prior output"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "occupied.txt"), []byte("prior content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "widget"), []byte("data"), 0o644))

	join := func(name string) string { return filepath.Join(workingDir, name) }

	cases := []struct {
		name       string
		command    string
		effect     Effect
		targets    []string
		recursive  bool
		unresolved bool
	}{
		{name: "rm tree", command: "rm -rf widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}, recursive: true},
		{name: "rm file", command: "rm widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}},
		{name: "rmdir", command: "rmdir widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}},
		{name: "unlink", command: "unlink widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}},
		{name: "shred", command: "shred -u widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}},
		{name: "git rm", command: "git rm widget", effect: EffectPermanentDeletion, targets: []string{join("widget")}},
		{name: "git clean", command: "git clean -fd", effect: EffectPermanentDeletion, recursive: true, unresolved: true},
		{name: "git clean dry run", command: "git clean -nd", effect: EffectNone},
		{name: "find delete", command: "find . -name '*.tmp' -delete", effect: EffectPermanentDeletion, targets: []string{workingDir}, recursive: true, unresolved: true},
		{name: "find exec rm", command: "find . -exec rm {} +", effect: EffectPermanentDeletion, targets: []string{workingDir}, recursive: true, unresolved: true},
		{name: "find list", command: "find . -name '*.tmp'", effect: EffectNone},
		{name: "truncate", command: "truncate -s 0 existing.log", effect: EffectPermanentDeletion, targets: []string{join("existing.log")}},
		{name: "truncating redirect", command: "echo hi > existing.log", effect: EffectPermanentDeletion, targets: []string{join("existing.log")}},
		{name: "appending redirect", command: "echo hi >> existing.log", effect: EffectNone},
		{name: "redirect to new file", command: "echo hi > new.log", effect: EffectNone},
		{name: "dd", command: "dd if=/dev/zero of=existing.log", effect: EffectPermanentDeletion, targets: []string{join("existing.log")}},
		{name: "nested shell", command: "bash -c 'rm -rf widget'", effect: EffectPermanentDeletion, targets: []string{join("widget")}, recursive: true},
		{name: "xargs", command: "xargs rm", effect: EffectPermanentDeletion, unresolved: true},
		{name: "quarantine move", command: "mv widget .quarantine/widget", effect: EffectQuarantine, targets: []string{join("widget")}},
		{name: "move onto existing file", command: "mv widget occupied.txt", effect: EffectPermanentDeletion, targets: []string{join("occupied.txt")}},
		{name: "read only", command: "echo hello", effect: EffectNone},
		{name: "version flag", command: "rm --version", effect: EffectNone},
		{name: "unrelated build", command: "go build ./...", effect: EffectNone},
		{name: "cd before removal", command: "cd sub && rm -rf widget", effect: EffectPermanentDeletion, recursive: true, unresolved: true},
		{name: "variable target", command: "rm -rf \"$TARGET\"", effect: EffectPermanentDeletion, recursive: true, unresolved: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			classification := Classify(tc.command, workingDir)
			require.Equal(t, tc.effect, classification.Effect, classification.Reason)
			require.Equal(t, tc.recursive, classification.Recursive, classification.Reason)
			require.Equal(t, tc.unresolved, classification.Unresolved != "", classification.Unresolved)
			if tc.targets != nil {
				require.Equal(t, CanonicalTargets(tc.targets), classification.Targets, classification.Reason)
			}
		})
	}
}

func TestDeletionDigestIsStable(t *testing.T) {
	t.Parallel()

	one := Digest([]string{"/a/b", "/a/c"})
	two := Digest([]string{"/a/c", "/a/b", "/a/b"})
	require.Equal(t, one, two)
	require.NotEqual(t, one, Digest([]string{"/a/b"}))
}

func TestDeletionLedgerRecordsHumanConfirmations(t *testing.T) {
	t.Parallel()

	ledger := NewLedger(t.TempDir())
	targets := []string{"/work/widget"}
	targetSet := Digest(targets)
	for stage := 1; stage <= 2; stage++ {
		require.NoError(t, ledger.AppendConfirmation("session", Confirmation{
			Stage: stage, TargetSet: targetSet, Targets: targets, Source: "human",
		}))
	}
	require.NoError(t, ledger.AppendExecution("session", Execution{
		TargetSet: targetSet, Targets: targets, ToolCallID: "call-1",
	}))

	record, err := ledger.Read("session")
	require.NoError(t, err)
	require.True(t, record.Confirmed(targetSet))
	execution, done := record.Executed(targetSet)
	require.True(t, done)
	require.Equal(t, "call-1", execution.ToolCallID)
}

func TestDeletionLedgerRejectsModelText(t *testing.T) {
	t.Parallel()

	ledger := NewLedger(t.TempDir())
	err := ledger.AppendConfirmation("session", Confirmation{
		Stage: 1, TargetSet: Digest([]string{"/work/widget"}), Targets: []string{"/work/widget"}, Source: "assistant",
	})
	require.Error(t, err)
}

func TestDeletionLedgerFailsClosedOnMalformedRecord(t *testing.T) {
	t.Parallel()

	ledger := NewLedger(t.TempDir())
	require.NoError(t, os.MkdirAll(ledger.Dir("session"), 0o755))
	require.NoError(t, os.WriteFile(ledger.Path("session"), []byte("{not json"), 0o644))

	_, err := ledger.Read("session")
	require.ErrorIs(t, err, ErrSessionRecordUnavailable)

	require.NoError(t, os.WriteFile(ledger.Path("session"), []byte(`{"schema":"other/v9","session_id":"session"}`), 0o644))
	_, err = ledger.Read("session")
	require.True(t, errors.Is(err, ErrSessionRecordUnavailable), err)
}

func TestDeletionLedgerMissingRecordIsEmpty(t *testing.T) {
	t.Parallel()

	record, err := NewLedger(t.TempDir()).Read("session")
	require.NoError(t, err)
	require.Empty(t, record.Confirmations)
	require.False(t, record.Confirmed(Digest([]string{"/work/widget"})))
}
