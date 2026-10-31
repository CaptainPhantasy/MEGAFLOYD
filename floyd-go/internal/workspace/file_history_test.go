package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/client"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestFileHistoryAndRewindLocalAndHTTP(t *testing.T) {
	h := newFrontendHarness(t)
	data := filepath.Join(h.workspace.Path, ".test-state")
	conn, err := db.Connect(t.Context(), data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(data)) })
	q := db.New(conn)
	h.app.Sessions = session.NewService(q, conn)
	h.app.Messages = message.NewService(q)
	h.app.History = history.NewService(q, conn)
	coord := &frontendControlCoordinator{busy: map[string]bool{}}
	h.app.AgentCoordinator = coord
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: data}}, h.workspace.Path)
	transport, err := client.NewClient(h.workspace.Path, "tcp", strings.TrimPrefix(h.httpSrv.URL, "http://"))
	require.NoError(t, err)
	local := workspace.NewAppWorkspace(h.app, store)
	remote := workspace.NewClientWorkspace(transport, proto.Workspace{ID: h.workspace.ID, Path: h.workspace.Path, Config: store.Config()})
	for _, test := range []struct {
		name string
		ws   workspace.Workspace
	}{{"local", local}, {"http", remote}} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			sess, err := test.ws.CreateSession(ctx, test.name)
			require.NoError(t, err)
			path := filepath.Join(h.workspace.Path, test.name+".txt")
			require.NoError(t, os.WriteFile(path, []byte("before"), 0o600))
			msg, err := h.app.Messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Edit the file"}}})
			require.NoError(t, err)
			recorder := h.app.History.(history.TurnRecorder)
			turn, err := recorder.BeginTurn(ctx, sess.ID, msg.ID, h.workspace.Path)
			require.NoError(t, err)
			require.NoError(t, history.CaptureBefore(turn, h.app.History, path))
			require.NoError(t, os.WriteFile(path, []byte("after"), 0o600))
			require.NoError(t, recorder.CompleteTurn(turn))
			versions, err := test.ws.ListSessionHistory(ctx, sess.ID)
			require.NoError(t, err)
			require.Len(t, versions, 2)
			coord.busy[sess.ID] = true
			_, err = test.ws.(workspace.FileRewindWorkspace).RewindSession(ctx, sess.ID, 1)
			require.Error(t, err, "an active agent turn must block file rewind")
			unchanged, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "after", string(unchanged))
			coord.busy[sess.ID] = false
			result, err := test.ws.(workspace.FileRewindWorkspace).RewindSession(ctx, sess.ID, 1)
			require.NoError(t, err)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "before", string(got))
			require.Len(t, result.RestoredFiles, 1)
			require.DirExists(t, result.RecoveryPath)
			messages, err := test.ws.ListMessages(ctx, sess.ID)
			require.NoError(t, err)
			require.Empty(t, messages)
			var records int
			require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM session_undo_records WHERE id = ?", result.Transcript.RecordID).Scan(&records))
			require.Equal(t, 1, records)
		})
	}
}
