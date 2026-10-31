package ralph

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoalSharedStateCannotOverwriteAnotherSession(t *testing.T) {
	dir := t.TempDir()
	a, b := New(dir), New(dir)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, entry := range []struct {
		loop    *Loop
		session string
	}{{a, "a"}, {b, "b"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := entry.loop.Start(entry.session, "finish work", 0, "")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.Contains(t, err.Error(), "already active")
		}
	}
	require.Equal(t, 1, successes)
	state, err := a.StatusChecked()
	require.NoError(t, err)
	require.NotNil(t, state)
	keep, _, _ := b.Check("different-session", "")
	require.False(t, keep)
	_, err = b.Cancel()
	require.NoError(t, err)
	require.False(t, a.IsActive())
	data, err := os.ReadFile(filepath.Join(dir, StateFileName))
	require.NoError(t, err)
	require.Contains(t, string(data), "active: false")
	_, err = a.Start("later", "new work", 0, "")
	require.NoError(t, err)
}

func TestGoalCorruptStateIsReportedAndPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StateFileName)
	before := []byte("active: [broken")
	require.NoError(t, os.WriteFile(path, before, 0o600))
	loop := New(dir)
	_, err := loop.StatusChecked()
	require.ErrorContains(t, err, "corrupt")
	_, err = loop.Start("session", "work", 0, "")
	require.ErrorContains(t, err, "corrupt")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestGoalCancellationChecksCurrentOwner(t *testing.T) {
	loop := New(t.TempDir())
	_, err := loop.Start("owner", "work", 0, "")
	require.NoError(t, err)
	_, err = loop.CancelSession("other")
	require.ErrorContains(t, err, "another session")
	require.True(t, loop.IsActive())
	_, err = loop.CancelSession("owner")
	require.NoError(t, err)
	require.False(t, loop.IsActive())
}
