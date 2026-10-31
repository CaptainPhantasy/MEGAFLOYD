package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestFileHistoryReportShowsVersionsAndExactContent(t *testing.T) {
	files := []history.File{{ID: "v-old", Path: "example.txt", Version: 0, Content: "before"}, {ID: "v-new", Path: "example.txt", Version: 1, Content: "complete new content\nlast line"}}
	list, err := fileHistoryReport(files, "")
	require.NoError(t, err)
	require.Contains(t, list, "v-old")
	require.Contains(t, list, "v-new")
	content, err := fileHistoryReport(files, "v-new")
	require.NoError(t, err)
	require.Contains(t, content, "complete new content\nlast line")
	_, err = fileHistoryReport(files, "missing")
	require.ErrorContains(t, err, "not found in this session")
}

func TestTypedFileHistoryAndRewindUseRealWorkspace(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	data := filepath.Join(root, ".test-state")
	conn, err := db.Connect(ctx, data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(data)) })
	a := app.NewForTest(ctx)
	t.Cleanup(a.ShutdownForTest)
	q := db.New(conn)
	a.Sessions = session.NewService(q, conn)
	a.Messages = message.NewService(q)
	a.History = history.NewService(q, conn)
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: data}}, root)
	ws := workspace.NewAppWorkspace(a, store)
	sess, err := ws.CreateSession(ctx, "TUI rewind")
	require.NoError(t, err)
	path := filepath.Join(root, "example.txt")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o600))
	msg, err := a.Messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Edit the file"}}})
	require.NoError(t, err)
	recorder := a.History.(history.TurnRecorder)
	turn, err := recorder.BeginTurn(ctx, sess.ID, msg.ID, root)
	require.NoError(t, err)
	require.NoError(t, history.CaptureBefore(turn, a.History, path))
	require.NoError(t, os.WriteFile(path, []byte("after"), 0o600))
	require.NoError(t, recorder.CompleteTurn(turn))
	u := newBusyUI(&countingWorkspace{ready: true})
	u.com.Workspace = ws
	u.session = &sess
	warmCaches(u, false)
	cmd, handled := u.dispatchSlashInput("/file-history")
	require.True(t, handled)
	require.NotNil(t, cmd)
	listed := cmd().(builtinResultMsg)
	require.NoError(t, listed.Err)
	require.Contains(t, listed.Body, "example.txt")
	require.Contains(t, listed.Body, "version 1")
	cmd, handled = u.dispatchSlashInput("/rewind 1")
	require.True(t, handled)
	require.NotNil(t, cmd)
	beforeRun, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "after", string(beforeRun), "UI Update must not perform file IO")
	result := cmd().(builtinResultMsg)
	require.NoError(t, result.Err)
	require.True(t, result.RefreshMessages)
	require.Empty(t, result.Messages)
	restored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "before", string(restored))
	require.Contains(t, result.Info, "recorded native file")
	u.Update(result)
}
