package session

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
)

func TestImportEnginePreservesPublishedHistoryAfterOutputReviewActivation(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	conn, err := db.Connect(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	queries := db.New(conn)
	sessions := NewService(queries, conn)
	messages := message.NewService(queries, message.WithDB(conn), message.WithDebounce(0))
	const secret = "test-only-import-secret"
	cfg := message.ReviewConfig{SecretValues: []string{secret}}
	require.NoError(t, message.EnableOutputReview(ctx, messages, cfg))

	userParts, err := message.MarshalParts([]message.ContentPart{message.TextContent{Text: "Retain this history."}})
	require.NoError(t, err)
	assistantParts, err := message.MarshalParts([]message.ContentPart{
		message.ReasoningContent{Thinking: "PRIVATE imported reasoning", Signature: "exact-imported-signature"},
		message.TextContent{Text: "Existing answer with " + secret + ". Keep `status --json` exact."},
		message.Finish{Reason: message.FinishReasonEndTurn, Time: 123},
	})
	require.NoError(t, err)
	transcript := []ImportMessage{{Role: "user", Parts: userParts}, {Role: "assistant", Parts: assistantParts}}
	imported, err := ImportEngine(ctx, sessions, "import-after-output-activation", "Existing history", transcript)
	require.NoError(t, err)
	require.Equal(t, 2, imported.Copied)
	stored, err := queries.ListMessagesBySession(ctx, imported.Session.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	require.Equal(t, string(assistantParts), stored[1].Parts, "provider originals must remain byte-exact")
	assistantID := stored[1].ID
	want := "Existing answer with [redacted]. Keep `status --json` exact."
	public, err := message.PublicGet(ctx, messages, assistantID)
	require.NoError(t, err)
	require.Equal(t, want, public.Content().Text, "imported history must not become an unpublished draft")
	require.Empty(t, public.ReasoningContent().Thinking)
	require.Empty(t, public.ReasoningContent().Signature)
	status, err := message.OutputReviewStatus(ctx, messages, assistantID)
	require.NoError(t, err)
	require.Equal(t, "imported_visible", status.State)
	require.Contains(t, string(status.Receipt), "imported-existing-history")
	final, err := message.FinalPublicText(ctx, messages, assistantID)
	require.NoError(t, err)
	require.Equal(t, want, final)
	replay, err := message.ReplayPublic(ctx, messages, imported.Session.ID, 0)
	require.NoError(t, err)
	require.Len(t, replay, 1)
	require.Equal(t, want, replay[0].Message.Content().Text)
	require.Empty(t, replay[0].Message.ReasoningContent().Thinking)
	var exported bytes.Buffer
	require.NoError(t, message.ExportPublic(ctx, messages, imported.Session.ID, &exported))
	require.NotContains(t, exported.String(), secret)
	require.NotContains(t, exported.String(), "PRIVATE")
	require.NotContains(t, exported.String(), "exact-imported-signature")

	again, err := ImportEngine(ctx, sessions, imported.Session.ID, "Unchanged", transcript)
	require.NoError(t, err)
	require.True(t, again.Existing)
	require.Zero(t, again.Copied)
	againEvents, err := message.ReplayPublic(ctx, messages, imported.Session.ID, 0)
	require.NoError(t, err)
	require.Equal(t, replay, againEvents)

	pendingParts, err := message.MarshalParts([]message.ContentPart{message.TextContent{Text: "Unpublished provider draft."}})
	require.NoError(t, err)
	_, err = queries.CreateMessage(ctx, db.CreateMessageParams{ID: "unrelated-pending", SessionID: imported.Session.ID, Role: "assistant", Parts: string(pendingParts)})
	require.NoError(t, err)
	require.NoError(t, db.Release(dir))
	reopened, err := db.Connect(ctx, dir)
	require.NoError(t, err)
	fresh := message.NewService(db.New(reopened), message.WithDB(reopened), message.WithDebounce(0))
	require.NoError(t, message.EnableOutputReview(ctx, fresh, cfg))
	visible, err := message.PublicGet(ctx, fresh, assistantID)
	require.NoError(t, err)
	require.Equal(t, want, visible.Content().Text)
	private, err := fresh.Get(ctx, assistantID)
	require.NoError(t, err)
	require.Equal(t, "exact-imported-signature", private.ReasoningContent().Signature)
	require.Contains(t, private.Content().Text, secret)
	pending, err := message.PublicGet(ctx, fresh, "unrelated-pending")
	require.NoError(t, err)
	require.Empty(t, pending.Content().Text, "explicit import must not grandfather unrelated pending drafts")
	reopenedEvents, err := message.ReplayPublic(ctx, fresh, imported.Session.ID, 0)
	require.NoError(t, err)
	require.Equal(t, replay, reopenedEvents)
}
