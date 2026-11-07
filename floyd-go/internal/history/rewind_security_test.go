//go:build darwin || linux

package history_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestRewindRollbackCannotFollowReplacedWorkspace(t *testing.T) {
	t.Parallel()
	data := t.TempDir()
	conn, err := db.Connect(t.Context(), data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(data)) })
	q := db.New(conn)
	stored, err := session.NewService(q, conn).Create(t.Context(), "rewind race")
	require.NoError(t, err)
	user, err := message.NewService(q).Create(t.Context(), stored.ID, message.CreateMessageParams{Role: message.User})
	require.NoError(t, err)
	files := history.NewService(q, conn)
	base := t.TempDir()
	root, outside, moved := filepath.Join(base, "workspace"), filepath.Join(base, "outside"), filepath.Join(base, "original-workspace")
	require.NoError(t, os.Mkdir(root, 0o700))
	require.NoError(t, os.Mkdir(outside, 0o700))
	path := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("before\n"), 0o600))
	recorder := files.(history.TurnRecorder)
	turn, err := recorder.BeginTurn(t.Context(), stored.ID, user.ID, root)
	require.NoError(t, err)
	require.NoError(t, history.CaptureBefore(turn, files, path))
	require.NoError(t, os.WriteFile(path, []byte("after\n"), 0o600))
	require.NoError(t, recorder.CompleteTurn(turn))
	outsideFile := filepath.Join(outside, "file.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("before\n"), 0o600))
	outsideInfo, err := os.Stat(outsideFile)
	require.NoError(t, err)
	_, _, err = files.(history.TurnRestorer).RestoreTurns(t.Context(), stored.ID, []string{user.ID}, root, func() error {
		operations, listErr := os.ReadDir(filepath.Join(root, ".quarantine", "rewind"))
		require.NoError(t, listErr)
		require.Len(t, operations, 1)
		// A separate process can prepare matching directory and file names.
		require.NoError(t, os.MkdirAll(filepath.Join(outside, ".quarantine", "rewind", operations[0].Name()), 0o700))
		require.NoError(t, os.Rename(root, moved))
		require.NoError(t, os.Symlink(outside, root))
		return errors.New("transcript commit refused")
	})
	require.ErrorContains(t, err, "transcript commit refused")
	content, err := os.ReadFile(outsideFile)
	require.NoError(t, err, "rollback must not move the substituted outside file")
	require.Equal(t, "before\n", string(content))
	actualInfo, err := os.Stat(outsideFile)
	require.NoError(t, err)
	require.True(t, os.SameFile(outsideInfo, actualInfo))
	content, err = os.ReadFile(filepath.Join(moved, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "after\n", string(content), "rollback must restore the original bound directory")
}
