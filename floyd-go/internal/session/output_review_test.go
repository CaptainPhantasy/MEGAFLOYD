package session_test

import (
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestOutputReviewClonePreservesPrivateAndPublicAcrossReopen(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	conn, err := db.Connect(ctx, dir)
	require.NoError(t, err)
	defer db.Release(dir)
	q := db.New(conn)
	sessions := session.NewService(q, conn)
	source, err := sessions.Create(ctx, "Source")
	require.NoError(t, err)
	messages := message.NewService(q, message.WithDB(conn), message.WithDebounce(0))
	review := message.ReviewConfig{SecretValues: []string{"actual-private-token"}}
	require.NoError(t, message.EnableOutputReview(ctx, messages, review))
	completed, err := messages.Create(ctx, source.ID, message.CreateMessageParams{Role: message.Assistant})
	require.NoError(t, err)
	completed.AppendContent("The actual-private-token value stays private.")
	completed.AppendReasoningContent("Private replay reasoning")
	completed.AppendReasoningSignature("signature-exact")
	completed.AddFinish(message.FinishReasonEndTurn, "", "")
	require.NoError(t, messages.Update(ctx, completed))
	pending, err := messages.Create(ctx, source.ID, message.CreateMessageParams{Role: message.Assistant})
	require.NoError(t, err)
	pending.AppendContent("Unfinished private draft")
	require.NoError(t, messages.Update(ctx, pending))
	before, err := message.PublicGet(ctx, messages, completed.ID)
	require.NoError(t, err)
	require.NotContains(t, before.Content().Text, "actual-private-token")
	fork, err := session.Fork(ctx, sessions, source.ID, -1)
	require.NoError(t, err)
	clone := fork.NewSession
	copied, err := messages.List(ctx, clone.ID)
	require.NoError(t, err)
	var completeID, pendingID string
	for _, item := range copied {
		if item.Role != message.Assistant {
			continue
		}
		if item.IsFinished() {
			completeID = item.ID
			require.Equal(t, completed.Parts, item.Parts)
		} else {
			pendingID = item.ID
		}
	}
	require.NotEmpty(t, completeID)
	require.NotEmpty(t, pendingID)
	for _, svc := range []message.Service{messages} {
		require.NoError(t, message.EnableOutputReview(ctx, svc, review))
		public, err := message.PublicGet(ctx, svc, completeID)
		require.NoError(t, err)
		require.Equal(t, before.Parts, public.Parts)
		public, err = message.PublicGet(ctx, svc, pendingID)
		require.NoError(t, err)
		require.Empty(t, public.Content().Text)
		require.Empty(t, public.ReasoningContent().Thinking)
	}
	events, err := message.ReplayPublic(ctx, messages, clone.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	for _, event := range events {
		require.Equal(t, clone.ID, event.Message.SessionID)
		require.NotContains(t, event.Message.Content().Text, "actual-private-token")
	}

	db.Release(dir)
	conn, err = db.Connect(ctx, dir)
	require.NoError(t, err)
	reopened := message.NewService(db.New(conn), message.WithDB(conn), message.WithDebounce(0))
	require.NoError(t, message.EnableOutputReview(ctx, reopened, review))
	public, err := message.PublicGet(ctx, reopened, completeID)
	require.NoError(t, err)
	require.Equal(t, before.Parts, public.Parts)
	public, err = message.PublicGet(ctx, reopened, pendingID)
	require.NoError(t, err)
	require.Empty(t, public.Content().Text)
	private, err := reopened.Get(ctx, completeID)
	require.NoError(t, err)
	require.Equal(t, completed.Parts, private.Parts)
}
