package session

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
)

func TestImportEngineIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	queries := db.New(conn)
	sessions := NewService(queries, conn)

	userParts, err := message.MarshalParts([]message.ContentPart{message.TextContent{Text: "hello"}})
	require.NoError(t, err)
	assistantParts, err := message.MarshalParts([]message.ContentPart{message.TextContent{Text: "hi"}})
	require.NoError(t, err)
	transcript := []ImportMessage{
		{Role: "user", Parts: userParts},
		{Role: "assistant", Parts: assistantParts},
	}

	first, err := ImportEngine(t.Context(), sessions, "session_import-1", "Imported", transcript)
	require.NoError(t, err)
	require.False(t, first.Existing)
	require.Equal(t, 2, first.Copied)
	require.Equal(t, "session_import-1", first.Session.ID)
	require.Equal(t, "Imported", first.Session.Title)

	stored, err := queries.ListMessagesBySession(t.Context(), "session_import-1")
	require.NoError(t, err)
	require.Len(t, stored, 2)

	second, err := ImportEngine(t.Context(), sessions, "session_import-1", "Imported", transcript)
	require.NoError(t, err)
	require.True(t, second.Existing)
	require.Zero(t, second.Copied)

	storedAgain, err := queries.ListMessagesBySession(t.Context(), "session_import-1")
	require.NoError(t, err)
	require.Len(t, storedAgain, 2)
}
