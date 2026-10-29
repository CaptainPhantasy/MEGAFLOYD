package filelock

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObserverLeaseKeepsReservationUntilShellRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target")
	owner, releaseOwner, err := AcquireWithContext(t.Context(), []string{path})
	require.NoError(t, err)
	defer releaseOwner()
	observer, finish := WithObserverLease(owner)
	childRelease, err := Acquire(observer, []string{path})
	require.NoError(t, err)
	defer childRelease()

	// Sibling shells borrow only the parent lease, never each other's lease.
	sibling, cancel := context.WithTimeout(observer, 25*time.Millisecond)
	_, err = Acquire(sibling, []string{path})
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	finished := make(chan struct{})
	go func() { finish(); close(finished) }()
	select {
	case <-finished:
		t.Fatal("parent resumed while observer shell still held its lease")
	case <-time.After(25 * time.Millisecond):
	}
	other, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	_, err = Acquire(other, []string{path})
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	childRelease()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("observer lease did not drain")
	}
	_, err = Acquire(observer, []string{path})
	require.ErrorIs(t, err, context.Canceled, "closed observer scopes cannot borrow again")
	other, cancel = context.WithTimeout(t.Context(), 25*time.Millisecond)
	_, err = Acquire(other, []string{path})
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded, "the parent still owns its reservation")
	releaseOwner()
	release, err := Acquire(t.Context(), []string{path})
	require.NoError(t, err)
	release()
}
