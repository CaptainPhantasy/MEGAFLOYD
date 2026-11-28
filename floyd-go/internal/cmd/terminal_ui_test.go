package cmd

import (
	"errors"
	"testing"

	"github.com/legacy-ai/floyd/internal/event"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestRootCommandOwnsTheTerminalWhileItRuns: the root command is the terminal
// interface. While it runs it reports owning the process terminal, so that
// nothing else writes to the screen, and it releases the terminal afterwards
// whether the run succeeds or fails.
func TestRootCommandOwnsTheTerminalWhileItRuns(t *testing.T) {
	t.Cleanup(func() { event.SetTerminalUI(false) })
	command := &cobra.Command{}
	state := func() (owned, active bool) { return event.TerminalUI(), tuiActive.Load() }

	succeeding := ownTerminalWhileRunning(func(*cobra.Command, []string) error {
		owned, active := state()
		require.True(t, owned, "the terminal interface owns the terminal while the root command runs")
		require.True(t, active)
		return nil
	})
	require.NoError(t, succeeding(command, nil))
	owned, active := state()
	require.False(t, owned, "and releases it afterwards")
	require.False(t, active)

	stopped := errors.New("terminal interface stopped")
	failing := ownTerminalWhileRunning(func(*cobra.Command, []string) error {
		require.True(t, event.TerminalUI())
		return stopped
	})
	require.ErrorIs(t, failing(command, nil), stopped)
	owned, active = state()
	require.False(t, owned, "a failed run releases the terminal too")
	require.False(t, active)
}
