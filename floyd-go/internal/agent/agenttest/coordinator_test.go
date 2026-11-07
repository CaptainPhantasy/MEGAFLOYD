package agenttest

import (
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestOfflineCoordinatorPersistsCancelledAcceptedTurn(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", filepath.Join(root, "config"))
	t.Setenv("FLOYD_GLOBAL_DATA", filepath.Join(root, "data"))
	t.Setenv("FLOYD_DISABLE_PROVIDER_AUTO_UPDATE", "1")
	t.Setenv("FLOYD_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("MEGAFLOYD_SKILLS_DIR", filepath.Join(root, "skills"))
	connection, err := db.Connect(t.Context(), filepath.Join(root, "db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(filepath.Join(root, "db"))) })
	queries := db.New(connection)
	sessions := session.NewService(queries, connection)
	messages := message.NewService(queries)
	coordinator, err := NewCoordinator(t.Context(), root, sessions, messages)
	require.NoError(t, err)
	sess, err := sessions.Create(t.Context(), "Offline cancellation fixture")
	require.NoError(t, err)
	accepted := coordinator.BeginAccepted(sess.ID)
	require.NotNil(t, accepted)
	coordinator.Cancel(sess.ID)
	_, err = coordinator.RunAccepted(t.Context(), accepted, sess.ID, "Do not contact any provider")
	require.NoError(t, err)
	require.False(t, coordinator.IsSessionBusy(sess.ID))
	persisted, err := messages.List(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Len(t, persisted, 2)
	require.Equal(t, message.User, persisted[0].Role)
	require.Equal(t, message.FinishReasonCanceled, persisted[1].FinishReason())
}
