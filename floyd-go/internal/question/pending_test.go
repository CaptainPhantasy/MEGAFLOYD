package question

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/stretchr/testify/require"
)

type pendingQuestionResult struct {
	answers []Answer
	err     error
}

func awaitPendingQuestion(t *testing.T, events <-chan pubsub.Event[Request]) Request {
	t.Helper()
	select {
	case event := <-events:
		return event.Payload
	case <-time.After(3 * time.Second):
		t.Fatal("question was not published")
		return Request{}
	}
}

func pendingHandoffQuestion() Request {
	return Request{
		SessionID: "shared-session", ToolCallID: "shared-tool-call",
		Questions: []Question{{
			Type: TypeSingleChoice, Text: "Continue in which interface?",
			Description: "Choose where to continue the shared session.",
			Choices:     []Choice{{ID: "terminal", Label: "Terminal"}, {ID: "browser", Label: "Browser"}},
		}},
	}
}

func TestPendingQuestionHandoffAndRequestID(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	service := NewService()
	firstInterface := service.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() {
		answers, err := service.Ask(ctx, pendingHandoffQuestion())
		result <- pendingQuestionResult{answers, err}
	}()
	original := awaitPendingQuestion(t, firstInterface)
	require.NotEmpty(t, original.ID)

	service.Subscribe(ctx)
	notifications := service.SubscribeNotifications(ctx)
	pending := service.Pending()
	require.Len(t, pending, 1)
	require.Equal(t, original, pending[0])
	answers := []Answer{{QuestionID: original.Questions[0].ID, SelectedIDs: []string{"browser"}}}
	require.False(t, service.AnswerRequest("stale-request", answers))
	require.False(t, service.CancelRequest("stale-request"))
	require.Len(t, service.Pending(), 1)
	require.True(t, service.AnswerRequest(original.ID, answers))
	require.Empty(t, service.Pending())
	require.False(t, service.CancelRequest(original.ID))

	select {
	case outcome := <-result:
		require.NoError(t, outcome.err)
		require.Equal(t, answers, outcome.answers)
	case <-ctx.Done():
		t.Fatal("answer did not release the original question")
	}
	select {
	case notification := <-notifications:
		require.Equal(t, original.ID, notification.Payload.BatchID)
	case <-ctx.Done():
		t.Fatal("late observer did not receive the question resolution")
	}
}

func TestPendingQuestionConcurrentAnswerCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	service := NewService()
	events := service.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() {
		answers, err := service.Ask(ctx, pendingHandoffQuestion())
		result <- pendingQuestionResult{answers, err}
	}()
	request := awaitPendingQuestion(t, events)
	answers := []Answer{{QuestionID: request.Questions[0].ID, SelectedIDs: []string{"browser"}}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var answerWon, cancelWon bool
	wg.Go(func() {
		<-start
		answerWon = service.AnswerRequest(request.ID, answers)
	})
	wg.Go(func() {
		<-start
		cancelWon = service.CancelRequest(request.ID)
	})
	close(start)
	wg.Wait()
	require.NotEqual(t, answerWon, cancelWon, "exactly one interface must resolve the question")
	require.Empty(t, service.Pending())
	require.False(t, service.AnswerRequest(request.ID, answers))
	require.False(t, service.CancelRequest(request.ID))

	select {
	case outcome := <-result:
		if answerWon {
			require.NoError(t, outcome.err)
			require.Equal(t, answers, outcome.answers)
		} else {
			require.ErrorIs(t, outcome.err, ErrCancelled)
			require.Empty(t, outcome.answers)
		}
	case <-ctx.Done():
		t.Fatal("winning resolution did not release the question")
	}
}

func TestPendingQuestionCancellationClearsSnapshot(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	service := NewService()
	events := service.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() {
		answers, err := service.Ask(ctx, pendingHandoffQuestion())
		result <- pendingQuestionResult{answers, err}
	}()
	request := awaitPendingQuestion(t, events)
	require.True(t, service.CancelRequest(request.ID))
	require.Empty(t, service.Pending())
	require.False(t, service.CancelRequest(request.ID))
	select {
	case outcome := <-result:
		require.ErrorIs(t, outcome.err, ErrCancelled)
	case <-ctx.Done():
		t.Fatal("cancellation did not release the question")
	}
}
