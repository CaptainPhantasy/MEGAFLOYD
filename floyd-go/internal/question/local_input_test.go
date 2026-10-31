package question

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func protectedQuestion() Request {
	request := pendingHandoffQuestion()
	request.RequireLocalInput = true
	return request
}

func TestRemoteServiceRejectsProtectedInputBeforePublishing(t *testing.T) {
	service := NewRemoteService()
	events := service.Subscribe(t.Context())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	answers, err := service.Ask(ctx, protectedQuestion())
	require.ErrorIs(t, err, ErrLocalInputRequired)
	require.Nil(t, answers)
	require.Empty(t, service.Pending())
	select {
	case <-events:
		t.Fatal("unsupported protected question was published")
	default:
	}
}

func TestRPCAnswerCannotSatisfyProtectedLocalQuestion(t *testing.T) {
	for _, withID := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-id", true: "exact-id"}[withID], func(t *testing.T) {
			service := NewService()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			events := service.Subscribe(ctx)
			result := make(chan pendingQuestionResult, 1)
			go func() {
				answers, err := service.Ask(ctx, protectedQuestion())
				result <- pendingQuestionResult{answers, err}
			}()
			request := awaitPendingQuestion(t, events)
			id := ""
			if withID {
				id = request.ID
			}
			yes := true
			require.False(t, service.AnswerRequest(id, []Answer{{QuestionID: request.Questions[0].ID, Yes: &yes}}))
			require.Empty(t, service.Pending())
			select {
			case outcome := <-result:
				require.ErrorIs(t, outcome.err, ErrLocalInputRequired)
				require.Empty(t, outcome.answers)
			case <-ctx.Done():
				t.Fatal("rejected RPC input left a protected confirmation hanging")
			}
		})
	}
}

func TestDirectLocalAnswerStillSatisfiesProtectedQuestion(t *testing.T) {
	service := NewService()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := service.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() {
		answers, err := service.Ask(ctx, protectedQuestion())
		result <- pendingQuestionResult{answers, err}
	}()
	request := awaitPendingQuestion(t, events)
	yes := true
	answers := []Answer{{QuestionID: request.Questions[0].ID, Yes: &yes}}
	require.True(t, service.Answer(answers))
	select {
	case outcome := <-result:
		require.NoError(t, outcome.err)
		require.Equal(t, answers, outcome.answers)
	case <-ctx.Done():
		t.Fatal("local input did not resolve protected question")
	}
}

func TestRemoteServicePreservesOrdinaryQuestions(t *testing.T) {
	service := NewRemoteService()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	events := service.Subscribe(ctx)
	result := make(chan pendingQuestionResult, 1)
	go func() {
		answers, err := service.Ask(ctx, pendingHandoffQuestion())
		result <- pendingQuestionResult{answers, err}
	}()
	request := awaitPendingQuestion(t, events)
	answers := []Answer{{QuestionID: request.Questions[0].ID, SelectedIDs: []string{"browser"}}}
	require.True(t, service.AnswerRequest(request.ID, answers))
	select {
	case outcome := <-result:
		require.NoError(t, outcome.err)
		require.Equal(t, answers, outcome.answers)
	case <-ctx.Done():
		t.Fatal("ordinary remote question did not resolve")
	}
}

func TestLocalInputMarkerIsNotAJSONAuthorityField(t *testing.T) {
	bytes, err := json.Marshal(protectedQuestion())
	require.NoError(t, err)
	var decoded Request
	require.NoError(t, json.Unmarshal(bytes, &decoded))
	require.False(t, decoded.RequireLocalInput)
	require.NoError(t, json.Unmarshal([]byte(`{"RequireLocalInput":true,"require_local_input":true}`), &decoded))
	require.False(t, decoded.RequireLocalInput)
}
