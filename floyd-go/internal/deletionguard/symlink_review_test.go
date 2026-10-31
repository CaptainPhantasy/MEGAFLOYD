package deletionguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymlinkReviewRejectsReferentReplacementDespiteExistingAnswers(t *testing.T) {
	for _, command := range []string{"> link", "printf replacement > link", "truncate -s 0 link"} {
		t.Run(command, func(t *testing.T) {
			guard, dir, _, confirmer := recoveryFixture(t)
			referent, link := filepath.Join(dir, "b"), filepath.Join(dir, "link")
			require.NoError(t, os.Symlink(referent, link))
			c := Classify(command, dir)
			require.True(t, c.IsDestructive())
			before, err := snapshotTargets(c.Targets)
			require.NoError(t, err)
			// Represent an old trusted approval made before the final-symlink gate.
			// Runtime workspace JSON is never imported to establish these answers.
			require.NoError(t, guard.saveApproval("historical-link", c, before))
			for stage := 1; stage <= 2; stage++ {
				require.NoError(t, guard.Ledger().AppendConfirmation("historical-link", Confirmation{Stage: stage, TargetSet: c.Digest(), Targets: c.Targets, Source: humanSource}))
			}
			require.NoError(t, os.Rename(referent, filepath.Join(dir, "quarantined-original-referent")))
			require.NoError(t, os.WriteFile(referent, []byte("new unapproved content"), 0o600))
			after, err := snapshotTargets(c.Targets)
			require.NoError(t, err)
			require.Equal(t, before, after, "the unchanged link entry cannot establish unchanged referent identity")
			restarted := testGuard(guard.ledger.dataDir, confirmer)
			calls := confirmer.calls
			decision := restarted.Check(t.Context(), "historical-link", "retry", c)
			require.False(t, decision.Allowed)
			require.Contains(t, decision.Reason, "symbolic link")
			require.Equal(t, calls, confirmer.calls, "no historical answer is reused or new question asked for an unsupported write")
			err = restarted.BeginExecution("historical-link", c, "launch")
			require.ErrorContains(t, err, "symbolic link")
			record, err := restarted.Ledger().Read("historical-link")
			require.NoError(t, err)
			require.Len(t, record.Confirmations, 2)
			require.Empty(t, record.Attempts)
			require.Empty(t, record.Executions)
			content, err := os.ReadFile(referent)
			require.NoError(t, err)
			require.Equal(t, "new unapproved content", string(content))
		})
	}
}
