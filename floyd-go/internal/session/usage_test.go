package session

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/stretchr/testify/require"
)

func TestCanonicalSuggestionUsagePreservesSessionState(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)); db.ResetPool() })
	service := NewService(db.New(conn), conn)
	sess, err := service.Create(t.Context(), "User chosen title")
	require.NoError(t, err)
	sess.PromptTokens = 100
	sess.CompletionTokens = 40
	sess.Cost = 1
	sess.Todos = []Todo{{Content: "Owner task", Status: TodoStatusInProgress}}
	sess.SummaryMessageID = "preserved-summary"
	sess, err = service.Save(t.Context(), sess)
	require.NoError(t, err)
	require.NoError(t, service.AddUsage(t.Context(), sess.ID, 7, 3, 0.25))
	got, err := service.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Equal(t, sess.Title, got.Title)
	require.Equal(t, sess.Todos, got.Todos)
	require.Equal(t, sess.SummaryMessageID, got.SummaryMessageID)
	require.EqualValues(t, 107, got.PromptTokens)
	require.EqualValues(t, 43, got.CompletionTokens)
	require.InDelta(t, 1.25, got.Cost, 0.000001)
}
