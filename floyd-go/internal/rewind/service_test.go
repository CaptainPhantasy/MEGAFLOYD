package rewind

import (
	"context"
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

type fixture struct {
	root, sid string
	sessions  session.Service
	messages  message.Service
	files     history.Service
	recorder  history.TurnRecorder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, ".test-state")
	conn, err := db.Connect(t.Context(), data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(data)) })
	q := db.New(conn)
	f := &fixture{root: root, sessions: session.NewService(q, conn), messages: message.NewService(q), files: history.NewService(q, conn)}
	f.recorder = f.files.(history.TurnRecorder)
	s, err := f.sessions.Create(t.Context(), "rewind fixture")
	require.NoError(t, err)
	f.sid = s.ID
	return f
}

func (f *fixture) turn(t *testing.T, changes map[string]string) string {
	t.Helper()
	msg, err := f.messages.Create(t.Context(), f.sid, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Make the requested changes"}}})
	require.NoError(t, err)
	ctx, err := f.recorder.BeginTurn(t.Context(), f.sid, msg.ID, f.root)
	require.NoError(t, err)
	for path, content := range changes {
		require.NoError(t, history.CaptureBefore(ctx, f.files, path))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	require.NoError(t, f.recorder.CompleteTurn(ctx))
	return msg.ID
}

func content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestRewindRestoresFilesAndTranscriptWithRecovery(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.root, "existing.txt")
	created := filepath.Join(f.root, "created.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	f.turn(t, map[string]string{path: "first edit"})
	f.turn(t, map[string]string{path: "second edit", created: "new file"})
	result, err := Rewind(t.Context(), f.sessions, f.messages, f.files, f.sid, 2, f.root)
	require.NoError(t, err)
	require.Equal(t, "original", content(t, path))
	require.NoFileExists(t, created)
	require.Len(t, result.RestoredFiles, 2)
	require.FileExists(t, filepath.Join(result.RecoveryPath, "manifest.json"))
	require.Equal(t, "new file", content(t, filepath.Join(result.RecoveryPath, "0.original")))
	require.Equal(t, "second edit", content(t, filepath.Join(result.RecoveryPath, "1.original")))
	remaining, err := f.messages.List(t.Context(), f.sid)
	require.NoError(t, err)
	require.Empty(t, remaining)
	require.NotEmpty(t, result.Transcript.RecordID)
	_, err = Rewind(t.Context(), f.sessions, f.messages, f.files, f.sid, 1, f.root)
	require.ErrorIs(t, err, session.ErrInvalidUndo)
}

func TestRewindRefusesChangedFilesAndKeepsTranscript(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.root, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	f.turn(t, map[string]string{path: "agent edit"})
	require.NoError(t, os.WriteFile(path, []byte("human edit"), 0o600))
	_, err := Rewind(t.Context(), f.sessions, f.messages, f.files, f.sid, 1, f.root)
	require.ErrorContains(t, err, "changed since")
	require.Equal(t, "human edit", content(t, path))
	remaining, err := f.messages.List(t.Context(), f.sid)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
}

func TestRewindRollsBackFilesWhenTranscriptCommitFails(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.root, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	mid := f.turn(t, map[string]string{path: "agent edit"})
	sentinel := errors.New("database commit failed")
	_, recovery, err := f.files.(history.TurnRestorer).RestoreTurns(t.Context(), f.sid, []string{mid}, f.root, func() error { require.Equal(t, "original", content(t, path)); return sentinel })
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, "agent edit", content(t, path))
	require.DirExists(t, recovery)
}

func TestRewindRefusesUnrecordedAndUnsupportedTurns(t *testing.T) {
	for _, mode := range []string{"unrecorded", "shell", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			path := filepath.Join(f.root, "file.txt")
			if mode == "unrecorded" {
				_, err := f.messages.Create(t.Context(), f.sid, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "old history"}}})
				require.NoError(t, err)
			} else {
				f.turn(t, map[string]string{path: "retained"})
			}
			if mode == "shell" {
				_, err := f.messages.Create(t.Context(), f.sid, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.ToolCall{ID: "shell", Name: "bash", Input: "{}"}}})
				require.NoError(t, err)
			}
			if mode == "symlink" {
				saved := filepath.Join(f.root, "saved.txt")
				require.NoError(t, os.Rename(path, saved))
				require.NoError(t, os.Symlink(saved, path))
			}
			_, err := Rewind(context.Background(), f.sessions, f.messages, f.files, f.sid, 1, f.root)
			require.Error(t, err)
			remaining, e := f.messages.List(t.Context(), f.sid)
			require.NoError(t, e)
			require.NotEmpty(t, remaining)
		})
	}
}

func TestRewindRollbackPreservesConcurrentEditsAndRestoresIndependentFiles(t *testing.T) {
	f := newFixture(t)
	a := filepath.Join(f.root, "a.txt")
	b := filepath.Join(f.root, "b.txt")
	require.NoError(t, os.WriteFile(a, []byte("a-before"), 0o600))
	require.NoError(t, os.WriteFile(b, []byte("b-before"), 0o600))
	mid := f.turn(t, map[string]string{a: "a-after", b: "b-after"})
	_, recovery, err := f.files.(history.TurnRestorer).RestoreTurns(t.Context(), f.sid, []string{mid}, f.root, func() error {
		require.NoError(t, os.WriteFile(b, []byte("concurrent human edit"), 0o600))
		return errors.New("commit failed")
	})
	require.ErrorContains(t, err, "recovery retained")
	require.Equal(t, "a-after", content(t, a), "independent rollback must continue after another file conflicts")
	require.Equal(t, "concurrent human edit", content(t, b))
	require.Equal(t, "b-after", content(t, filepath.Join(recovery, "1.original")))
}
