package frontendops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/enginebridge"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestFrontendSessionExportImportRetainsMessagesAndIsIdempotent(t *testing.T) {
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	t.Setenv("MEGAFLOYD_WEB_DATA", t.TempDir())
	root, dir := t.TempDir(), t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	q := db.New(conn)
	env := Environment{Config: config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: dir}}, root), Sessions: session.NewService(q, conn), Messages: message.NewService(q)}
	original, err := env.Sessions.Create(t.Context(), "Portable fixture")
	require.NoError(t, err)
	_, err = env.Messages.Create(t.Context(), original.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Keep this exact text."}}})
	require.NoError(t, err)
	exported, err := Run(t.Context(), env, proto.FrontendRequest{Kind: "session", Action: "export", SessionID: original.ID})
	require.NoError(t, err)
	require.DirExists(t, exported.Path)
	found, err := enginebridge.Find(enginebridge.Home(), exported.SessionID)
	require.NoError(t, err)
	require.Equal(t, "Portable fixture", found.Title)
	transcript, err := enginebridge.ReadTranscript(found.Dir)
	require.NoError(t, err)
	require.Len(t, transcript, 1)
	require.Equal(t, "user", transcript[0].Role)
	imported, err := Run(t.Context(), env, proto.FrontendRequest{Kind: "session", Action: "import", Argument: exported.SessionID})
	require.NoError(t, err)
	messages, err := env.Messages.List(t.Context(), imported.SessionID)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "Keep this exact text.", messages[0].Content().Text)
	repeated, err := Run(t.Context(), env, proto.FrontendRequest{Kind: "session", Action: "import", Argument: exported.SessionID})
	require.NoError(t, err)
	require.Equal(t, imported.SessionID, repeated.SessionID)
	require.Contains(t, repeated.Body, "Already imported: true")
	originals, err := env.Messages.List(t.Context(), original.ID)
	require.NoError(t, err)
	require.Len(t, originals, 1)
}

func TestFrontendOperationsValidateInputAndUseSelectedWorkspace(t *testing.T) {
	t.Parallel()
	root, dir := t.TempDir(), t.TempDir()
	env := Environment{Config: config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: dir}}, root)}
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("needle-for-selected-workspace"), 0o600))
	result, err := Run(t.Context(), env, proto.FrontendRequest{Kind: "codebase", Action: "search", Argument: "needle-for-selected-workspace"})
	require.NoError(t, err)
	require.Contains(t, result.Body, "fixture.txt")
	result, err = Run(t.Context(), env, proto.FrontendRequest{Kind: "memory"})
	require.NoError(t, err)
	require.Contains(t, result.Body, "No memory checkpoint exists")
	require.Equal(t, dir, filepath.Dir(result.Path))
	for _, in := range []proto.FrontendRequest{{Kind: "unknown"}, {Kind: "codebase", Action: "search"}, {Kind: "session", Action: "import"}, {Kind: "hooks", Action: "trace", Limit: -1}} {
		_, err := Run(t.Context(), env, in)
		require.Error(t, err)
	}
	_, err = Run(t.Context(), Environment{}, proto.FrontendRequest{Kind: "doctor"})
	require.ErrorContains(t, err, "configuration is unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Run(ctx, env, proto.FrontendRequest{Kind: "codebase", Action: "search", Argument: "needle"})
	require.ErrorIs(t, err, context.Canceled)
}
