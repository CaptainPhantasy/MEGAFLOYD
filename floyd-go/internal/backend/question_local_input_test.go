package backend

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/stretchr/testify/require"
)

func TestBackendRPCQuestionAnswerCannotConfirmLocalDeletion(t *testing.T) {
	b, _ := newTestBackend(t)
	ws, _ := insertTestWorkspace(t, b, t.TempDir())
	service := question.NewService()
	ws.App = &app.App{Questions: service}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := service.Subscribe(ctx)
	result := make(chan error, 1)
	go func() {
		_, err := service.Ask(ctx, question.Request{RequireLocalInput: true, Questions: []question.Question{{Type: question.TypeYesNo, Text: "Confirm permanent deletion?", Description: "Protected input fixture."}}})
		result <- err
	}()
	select {
	case event := <-events:
		yes := true
		accepted, err := b.AnswerQuestion(ws.ID, proto.QuestionAnswer{BatchRequestID: event.Payload.ID, Responses: []proto.QuestionResponse{{QuestionID: event.Payload.Questions[0].ID, Yes: &yes}}})
		require.NoError(t, err)
		require.False(t, accepted)
	case <-ctx.Done():
		t.Fatal("protected question fixture did not publish")
	}
	select {
	case err := <-result:
		require.ErrorIs(t, err, question.ErrLocalInputRequired)
	case <-ctx.Done():
		t.Fatal("RPC rejection left the confirmation hanging")
	}
	require.Empty(t, service.Pending())
}

func TestBackendCreatesRemoteOnlyQuestionService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	b := New(t.Context(), nil, func() {})
	b.SetCreateGrace(2 * time.Second)
	t.Cleanup(func() { drainBackend(t, b) })
	ws, _, err := b.CreateWorkspace(protoWS(t.TempDir(), t.TempDir(), uuid.NewString()))
	require.NoError(t, err)
	events := ws.Questions.Subscribe(t.Context())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = ws.Questions.Ask(ctx, question.Request{RequireLocalInput: true, Questions: []question.Question{{Type: question.TypeYesNo, Text: "Confirm permanent deletion?", Description: "Protected constructor fixture."}}})
	require.ErrorIs(t, err, question.ErrLocalInputRequired)
	require.Empty(t, ws.Questions.Pending())
	select {
	case <-events:
		t.Fatal("backend created a service that published unsupported local confirmation")
	default:
	}
}
