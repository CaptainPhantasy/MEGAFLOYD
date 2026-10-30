package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/stretchr/testify/require"
)

func TestHTTPAnswerCannotConfirmProtectedLocalQuestion(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := h.app.Questions.Subscribe(ctx)
	type outcome struct {
		answers []question.Answer
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		answers, err := h.app.Questions.Ask(ctx, question.Request{
			ID: "protected-local-batch", SessionID: "protected-session", ToolCallID: "protected-call", RequireLocalInput: true,
			Questions: []question.Question{{ID: "protected-confirmation", Type: question.TypeYesNo, Text: "Confirm permanent deletion?", Description: "Protected input fixture; no operation executes."}},
		})
		done <- outcome{answers: answers, err: err}
	}()
	var request question.Request
	select {
	case event := <-events:
		request = event.Payload
	case <-ctx.Done():
		t.Fatal("protected local question did not publish")
	}
	require.True(t, request.RequireLocalInput)
	yes := true
	body, err := json.Marshal(proto.QuestionAnswer{BatchRequestID: request.ID, Responses: []proto.QuestionResponse{{QuestionID: request.Questions[0].ID, Yes: &yes}}})
	require.NoError(t, err)
	target := h.httpSrv.URL + "/v1/workspaces/" + h.workspace.ID + "/questions/answer"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := h.httpSrv.Client().Do(httpRequest)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var resolved proto.QuestionAnswerResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&resolved))
	require.False(t, resolved.Resolved, "HTTP input cannot authorize a protected local confirmation")
	select {
	case result := <-done:
		require.ErrorIs(t, result.err, question.ErrLocalInputRequired)
		require.Empty(t, result.answers, "rejected HTTP input must produce no confirmation answer")
	case <-ctx.Done():
		t.Fatal("HTTP rejection left the asking service blocked")
	}
	require.Empty(t, h.app.Questions.Pending())
}
