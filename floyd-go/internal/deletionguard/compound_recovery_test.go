package deletionguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompoundTerminalUnlinkPreservesExactRemainder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	confirmer := &recoveryConfirmer{}
	guard := testGuard(t.TempDir(), confirmer)
	all := ClassifyAll([]string{`truncate -s 0 b; rm b`, `rm a`}, dir)
	decision := guard.Check(t.Context(), "compound", "one", all)
	require.True(t, decision.Allowed, decision.Reason)
	require.Equal(t, 2, confirmer.calls)
	require.NoError(t, guard.BeginExecution("compound", all, "one"))
	b := filepath.Join(dir, "b")
	// Renaming into private quarantine simulates the terminal absence while
	// preserving the fixture's bytes. No truncate or unlink command executes.
	require.NoError(t, os.Rename(b, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, guard.RecordOutcome("compound", all, "one", []string{b}, false))
	record, err := guard.Ledger().Read("compound")
	require.NoError(t, err)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{b}, record.Executions[0].Targets)
	require.Equal(t, "finished", record.Attempts[len(record.Attempts)-1].State)
	restarted := testGuard(guard.ledger.dataDir, nil)
	remaining := Classify(`rm a`, dir)
	decision = restarted.Check(t.Context(), "compound", "retry", remaining)
	require.True(t, decision.Allowed, decision.Reason)
	require.NoError(t, restarted.BeginExecution("compound", remaining, "retry"))
	record, err = restarted.Ledger().Read("compound")
	require.NoError(t, err)
	require.Len(t, record.Confirmations, 2)
	require.Len(t, record.Approvals, 1)
	require.Equal(t, all.Targets, record.Approvals[0].Targets)
	require.False(t, restarted.Check(t.Context(), "compound", "repeat", all).Allowed)
	data, err := os.ReadFile(filepath.Join(dir, "quarantined-b"))
	require.NoError(t, err)
	require.Equal(t, "b", string(data))
}

func TestCompoundAbsenceWithoutTerminalUnlinkIsNotCompletion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(b, []byte("original"), 0o600))
	guard := testGuard(t.TempDir(), &recoveryConfirmer{})
	c := Classify(`truncate -s 0 b`, dir)
	require.True(t, guard.Check(t.Context(), "absence", "one", c).Allowed)
	require.NoError(t, guard.BeginExecution("absence", c, "one"))
	require.NoError(t, os.Rename(b, filepath.Join(dir, "quarantined-b")))
	require.NoError(t, guard.RecordOutcome("absence", c, "one", []string{b}, false))
	record, err := guard.Ledger().Read("absence")
	require.NoError(t, err)
	require.Empty(t, record.Executions)
	require.Equal(t, "uncertain", record.Attempts[len(record.Attempts)-1].State)
}

func TestCompoundUnsupportedTerminalTransformRejectedBeforeApproval(t *testing.T) {
	t.Parallel()
	for _, command := range []string{`rm b; truncate -s 0 b`, `truncate -s 0 b; shred b`} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("original"), 0o600))
			confirmer := &recoveryConfirmer{}
			guard := testGuard(t.TempDir(), confirmer)
			c := Classify(command, dir)
			require.True(t, c.IsDestructive())
			require.False(t, guard.Check(t.Context(), "unsupported", "one", c).Allowed)
			require.Zero(t, confirmer.calls)
			require.Error(t, guard.BeginExecution("unsupported", c, "one"))
			record, err := guard.Ledger().Read("unsupported")
			require.NoError(t, err)
			require.Empty(t, record.Confirmations)
			require.Empty(t, record.Attempts)
		})
	}
}

func TestCompoundTerminalRemovalUsesOrderedValidDescriptors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		signature string
		terminal  bool
		all       bool
	}{
		{`[["truncate","-s","0"],["/bin/rm"]]`, true, false},
		{`[["rm"],["unlink"]]`, true, true},
		{`[["rmdir"]]`, true, true},
		{`[["rm"],["truncate","-s","0"]]`, false, false},
		{`[["rm"],["shred"]]`, false, false},
		{`[["rm"],[]]`, false, false},
		{`[[],["rm"]]`, false, false},
		{`[[""],["rm"]]`, false, false},
		{`[[null],["rm"]]`, false, false},
		{`[]`, false, false},
		{`null`, false, false},
		{`invalid`, false, false},
	} {
		c := Classification{Operations: map[string]string{"target": tc.signature}}
		require.Equal(t, tc.terminal, c.TerminalRemovesTarget("target"), tc.signature)
		require.Equal(t, tc.all, c.RemovesTarget("target"), tc.signature)
	}
}
