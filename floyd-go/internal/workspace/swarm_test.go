package workspace

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

// TestAppWorkspaceSwarmRoundTrip checks the local path end to end: the
// UI writes through the workspace, and the value lands in the session
// metadata that the agent coordinator reads when it gates subagents.
func TestAppWorkspaceSwarmRoundTrip(t *testing.T) {
	t.Parallel()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	sessions := session.NewService(db.New(conn), conn)
	sess, err := sessions.Create(t.Context(), "Swarm")
	require.NoError(t, err)

	w := NewAppWorkspace(&app.App{Sessions: sessions}, nil)

	st, err := w.Swarm(t.Context(), sess.ID)
	require.NoError(t, err)
	require.False(t, st.Active, "swarm must start disabled")
	require.Equal(t, 3, st.MaxParallel, "default parallelism must be three")

	active := true
	st, err = w.SetSwarm(t.Context(), sess.ID, &active, nil)
	require.NoError(t, err)
	require.True(t, st.Active)

	// A second read must come from durable session metadata, not a field
	// cached in the workspace.
	st, err = w.Swarm(t.Context(), sess.ID)
	require.NoError(t, err)
	require.True(t, st.Active)

	parallel := 6
	st, err = w.SetSwarm(t.Context(), sess.ID, nil, &parallel)
	require.NoError(t, err)
	require.True(t, st.Active, "changing the ceiling must not disable swarm")
	require.Equal(t, 6, st.MaxParallel)

	reloaded, err := sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	swarm, ok := reloaded.Metadata["swarm"].(map[string]any)
	require.True(t, ok, "swarm state must persist in session metadata")
	require.Equal(t, true, swarm["active"])
	require.Equal(t, float64(6), swarm["max_parallel"])
}
