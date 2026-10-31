package model

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

type storedCommandWorkspace struct {
	workspace.Workspace
	cfg *config.Config
}

func (w *storedCommandWorkspace) Config() *config.Config { return w.cfg }
func (w *storedCommandWorkspace) WorkingDir() string     { return w.cfg.Options.DataDirectory }
func (w *storedCommandWorkspace) RenameSession(ctx context.Context, sid, title string) error {
	return w.Workspace.(workspace.SessionTitleWorkspace).RenameSession(ctx, sid, title)
}

func TestTitleAndUndoUpdateRealStoreAndVisibleConversation(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	conn, err := db.Connect(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	queries := db.New(conn)
	sessions := session.NewService(queries, conn)
	messages := message.NewService(queries)
	sess, err := sessions.Create(ctx, "Original")
	require.NoError(t, err)
	_, err = messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "WITHDRAW_ME"}}})
	require.NoError(t, err)
	_, err = messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "WITHDRAW_RESULT"}}})
	require.NoError(t, err)
	cfg := testConfigWithModel(1000)
	cfg.Options.DataDirectory = dir
	ws := &storedCommandWorkspace{Workspace: workspace.NewAppWorkspace(&app.App{Sessions: sessions, Messages: messages}, nil), cfg: cfg}
	m := newTestUI()
	m.com.Workspace = ws
	m.dialog = dialog.NewOverlay()
	m.session = &sess
	stored, err := ws.ListMessages(ctx, sess.ID)
	require.NoError(t, err)
	m.setSessionMessages(stored)
	renameCmd := m.titleCommand("Visible renamed title")
	require.NoError(t, sessions.AddUsage(ctx, sess.ID, 730, 90, 1.25))
	rename := renameCmd()
	m.Update(rename)
	require.Equal(t, "Visible renamed title", m.session.Title)
	saved, err := sessions.Get(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, m.session.Title, saved.Title)
	require.EqualValues(t, 730, saved.PromptTokens, "renaming a stale visible session must preserve newer usage")
	require.EqualValues(t, 90, saved.CompletionTokens)
	require.Equal(t, 1.25, saved.Cost)
	require.EqualValues(t, 2, saved.MessageCount)
	result, ok := m.undoPrompts("1")().(builtinResultMsg)
	require.True(t, ok)
	require.NoError(t, result.Err)
	require.True(t, result.RefreshMessages)
	m.Update(result)
	require.Equal(t, 0, m.chat.list.Len())
	remaining, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Empty(t, remaining)
	var backups int
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT count(*) FROM session_undo_records WHERE session_id = ?", sess.ID).Scan(&backups))
	require.Equal(t, 1, backups)
}

func TestProviderMenuBackRestoresProviderAndPalette(t *testing.T) {
	m, overlay := newBuiltinTestUI(testConfigWithModel(1000))
	palette, err := dialog.NewCommands(m.com, "sess-1", true, false, false, nil, nil)
	require.NoError(t, err)
	overlay.OpenDialog(palette)
	for _, r := range "provider" {
		m.handleDialogMsg(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, providersDialogID, overlay.FrontID())
	parent := overlay.DialogLast()
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, providerModelsDialogID, overlay.FrontID())
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	require.Same(t, parent, overlay.DialogLast())
	m.handleDialogMsg(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	require.Same(t, palette, overlay.DialogLast())
}

func TestPromptTemplatePreservesDraftAndRendersValues(t *testing.T) {
	m := newTestUI()
	m.dialog = dialog.NewOverlay()
	m.textarea.SetValue("Existing draft")
	cmd := m.promptTemplateCommand(`explainCode {"code":"answer := 42"}`)
	require.NotNil(t, cmd)
	require.Contains(t, m.textarea.Value(), "Existing draft")
	require.Contains(t, m.textarea.Value(), "Please explain what this code does:")
	require.Contains(t, m.textarea.Value(), "answer := 42")
}
