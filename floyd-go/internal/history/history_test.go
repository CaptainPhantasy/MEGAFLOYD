package history_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestFileVersionsAndTurnRewindPreserveHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	q := db.New(conn)
	sessions := session.NewService(q, conn)
	stored, err := sessions.Create(t.Context(), "fixture")
	require.NoError(t, err)
	files := history.NewService(q, conn)
	initial, err := files.Create(t.Context(), stored.ID, "versioned.txt", "before")
	require.NoError(t, err)
	updated, err := files.CreateVersion(t.Context(), stored.ID, "versioned.txt", "after")
	require.NoError(t, err)
	require.Equal(t, initial.Version+1, updated.Version)
	latest, err := files.GetByPathAndSession(t.Context(), "versioned.txt", stored.ID)
	require.NoError(t, err)
	require.Equal(t, "after", latest.Content)
	original, err := files.Get(t.Context(), initial.ID)
	require.NoError(t, err)
	require.Equal(t, "before", original.Content)
	messages := message.NewService(q)
	user, err := messages.Create(t.Context(), stored.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "change fixture"}}})
	require.NoError(t, err)
	root := t.TempDir()
	path := filepath.Join(root, "editable.txt")
	require.NoError(t, os.WriteFile(path, []byte("before\n"), 0o600))
	recorder := files.(history.TurnRecorder)
	turn, err := recorder.BeginTurn(t.Context(), stored.ID, user.ID, root)
	require.NoError(t, err)
	require.NoError(t, history.CaptureBefore(turn, files, path))
	require.NoError(t, os.WriteFile(path, []byte("after\n"), 0o600))
	require.NoError(t, recorder.CompleteTurn(turn))
	require.NoError(t, recorder.CompleteTurn(turn))
	turns, err := recorder.ListTurns(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, []history.Turn{{TurnID: 0, UserMessageID: user.ID, Recorded: true}}, turns)
	changes, err := recorder.TurnChanges(t.Context(), stored.ID, 0, root)
	require.NoError(t, err)
	require.True(t, changes.Recorded)
	require.Len(t, changes.Changes, 1)
	require.Equal(t, "editable.txt", changes.Changes[0].Path)
	start, err := recorder.TurnContent(t.Context(), stored.ID, 0, path, "start", root)
	require.NoError(t, err)
	require.Equal(t, "before\n", *start.Content)
	end, err := recorder.TurnContent(t.Context(), stored.ID, 0, path, "end", root)
	require.NoError(t, err)
	require.Equal(t, "after\n", *end.Content)
	require.NoError(t, os.WriteFile(path, []byte("human edit\n"), 0o600))
	called := false
	_, _, err = files.(history.TurnRestorer).RestoreTurns(t.Context(), stored.ID, []string{user.ID}, root, func() error { called = true; return nil })
	require.Error(t, err)
	require.False(t, called)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "human edit\n", string(content))
	require.NoError(t, os.WriteFile(path, []byte("after\n"), 0o600))
	// Rebuild the service against the same DB to exercise persisted checkpoints.
	restarted := history.NewService(q, conn)
	paths, recovery, err := restarted.(history.TurnRestorer).RestoreTurns(t.Context(), stored.ID, []string{user.ID}, root, func() error { called = true; return nil })
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []string{path}, paths)
	require.DirExists(t, recovery)
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "before\n", string(content))
	original, err = restarted.Get(t.Context(), initial.ID)
	require.NoError(t, err)
	require.Equal(t, "before", original.Content)
}
