package workspace_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/client"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestRenameSessionLocalAndHTTPPreserveUsageAndMetadata(t *testing.T) {
	h := newFrontendHarness(t)
	data := filepath.Join(h.workspace.Path, ".test-state")
	conn, err := db.Connect(t.Context(), data)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(data)) })
	h.app.Sessions = session.NewService(db.New(conn), conn)
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
			sess, err := test.ws.CreateSession(ctx, "Original")
			require.NoError(t, err)
			sess.Metadata = map[string]any{"owner_note": "preserved"}
			sess.Todos = []session.Todo{{Content: "Continue", Status: session.TodoStatusPending}}
			_, err = h.app.Sessions.Save(ctx, sess)
			require.NoError(t, err)
			require.NoError(t, h.app.Sessions.AddUsage(ctx, sess.ID, 730, 90, 1.25))
			before, err := h.app.Sessions.Get(ctx, sess.ID)
			require.NoError(t, err)
			require.NoError(t, test.ws.(workspace.SessionTitleWorkspace).RenameSession(ctx, sess.ID, "Changed title"))
			after, err := h.app.Sessions.Get(ctx, sess.ID)
			require.NoError(t, err)
			before.Title = "Changed title"
			require.Equal(t, before, after, "only the title may change")
		})
	}
}
