package shell

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCanonicalBackgroundTaskOwnershipAndCancellation(t *testing.T) {
	m := newBackgroundShellManager()
	dir := t.TempDir()
	job, err := m.StartForSession(t.Context(), dir, nil, "printf canonical-task", "direct check", "session-a", dir)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.True(t, job.WaitContext(ctx))
	stdout, _, done, err := job.GetOutput()
	require.NoError(t, err)
	require.True(t, done)
	require.Equal(t, "canonical-task", stdout)
	require.Equal(t, "session-a", job.SessionID)
	require.Equal(t, dir, job.WorkspaceDir)
	require.False(t, job.StartedAt.IsZero())
	require.False(t, job.CompletedAt().IsZero())
	job, err = m.StartForSession(t.Context(), dir, nil, "sleep 30", "cancel check", "session-b", dir)
	require.NoError(t, err)
	require.NoError(t, m.Cancel(job.ID))
	require.True(t, job.WaitContext(ctx))
	require.True(t, job.Cancelled())
	_, exists := m.Get(job.ID)
	require.True(t, exists, "cancelled task stays inspectable")
}
