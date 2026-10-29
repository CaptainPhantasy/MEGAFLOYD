package backend

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestOutputReviewBackendReadsOnlySQLPublicProjection(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	defer db.Release(dir)
	q := db.New(conn)
	sessions := session.NewService(q, conn)
	se, err := sessions.Create(t.Context(), "public-read")
	require.NoError(t, err)
	messages := message.NewService(q, message.WithDB(conn), message.WithDebounce(0))
	require.NoError(t, message.EnableOutputReview(t.Context(), messages, message.ReviewConfig{}))
	b, _ := newTestBackend(t)
	ws, _ := insertTestWorkspace(t, b, t.TempDir())
	ws.App = &app.App{Messages: messages, Sessions: sessions}
	raw, err := messages.Create(t.Context(), se.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "Unfinished draft"}, message.ReasoningContent{Thinking: "PRIVATE", Signature: "exact-signature"}}})
	require.NoError(t, err)
	require.NoError(t, messages.Update(t.Context(), raw))
	before, err := b.ListSessionMessages(t.Context(), ws.ID, se.ID)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Empty(t, before[0].Content().Text)
	require.Empty(t, before[0].ReasoningContent().Thinking)
	raw.AppendContent(". ")
	raw.AddFinish(message.FinishReasonEndTurn, "", "")
	require.NoError(t, messages.Update(t.Context(), raw))
	after, err := b.ListSessionMessages(t.Context(), ws.ID, se.ID)
	require.NoError(t, err)
	require.Equal(t, "Unfinished draft. ", after[0].Content().Text)
	require.Empty(t, after[0].ReasoningContent().Thinking)
	original, err := messages.Get(t.Context(), raw.ID)
	require.NoError(t, err)
	require.Equal(t, "Unfinished draft. ", original.Content().Text)
	require.Equal(t, "exact-signature", original.ReasoningContent().Signature)
}
