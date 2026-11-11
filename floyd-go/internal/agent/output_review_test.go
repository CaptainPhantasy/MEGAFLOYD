package agent

import (
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func reviewProjectionService(t *testing.T, enableReview bool, cfg message.ReviewConfig) (message.Service, string) {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Release(dir) })
	q := db.New(conn)
	se, err := session.NewService(q, conn).Create(t.Context(), "output review projection")
	require.NoError(t, err)
	svc := message.NewService(q, message.WithDB(conn), message.WithDebounce(0))
	if enableReview {
		require.NoError(t, message.EnableOutputReview(t.Context(), svc, cfg))
	}
	return svc, se.ID
}

func createAssistantMessage(t *testing.T, svc message.Service, sid, text string) string {
	t.Helper()
	msg, err := svc.Create(t.Context(), sid, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: text}},
	})
	require.NoError(t, err)
	msg.AddFinish(message.FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(t.Context(), msg))
	return msg.ID
}

// Regression for the FatChin finding on publicOutputResult: when the public
// projection is unavailable (review pending or withheld), the deferred Run
// wrapper used to replace the result with a content-stripped copy and return
// it as a successful blank answer. The result must keep the run's own
// content; public-facing surfaces stay gated by the review state because
// they read through FinalPublicText/PublicGet, not this result object.
func TestPublicOutputResultKeepsContentWhenReviewPending(t *testing.T) {
	t.Parallel()

	svc, sid := reviewProjectionService(t, true, message.ReviewConfig{RequireFinalReview: true})
	msgID := createAssistantMessage(t, svc, sid, "under review")

	base := &fantasy.AgentResult{}
	base.Response.Content = fantasy.ResponseContent{fantasy.TextContent{Text: "under review"}}
	base.Steps = []fantasy.StepResult{{}}

	agent := &sessionAgent{messages: svc}
	out := agent.publicOutputResult(t.Context(), base, msgID)

	_, err := message.FinalPublicText(t.Context(), svc, msgID)
	require.ErrorIs(t, err, message.ErrOutputReviewPending, "precondition: projection must be pending")
	require.NotEmpty(t, out.Response.Content, "pending review must not blank the run result")
	require.Equal(t, "under review", out.Response.Content.Text())
	require.Nil(t, out.Steps, "projection still strips intermediate steps")
}

func TestPublicOutputResultUsesProjectionWhenVisible(t *testing.T) {
	t.Parallel()

	svc, sid := reviewProjectionService(t, false, message.ReviewConfig{})
	msgID := createAssistantMessage(t, svc, sid, "published answer")

	base := &fantasy.AgentResult{}
	base.Response.Content = fantasy.ResponseContent{fantasy.TextContent{Text: "stale stream copy"}}

	agent := &sessionAgent{messages: svc}
	out := agent.publicOutputResult(t.Context(), base, msgID)

	require.Equal(t, "published answer", out.Response.Content.Text())
	require.Nil(t, out.Steps)
}
