package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/stretchr/testify/require"
)

func TestHTTPQuestionAnswerAcknowledgesDurableDeliveryAndRetry(t *testing.T) {
	h := newE2EHarness(t)
	dir := t.TempDir()
	question.ConfigureStorage(h.app.Questions, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := h.app.Questions.Subscribe(ctx)
	answers := make(chan []question.Answer, 1)
	go func() {
		a, _ := h.app.Questions.Ask(ctx, question.Request{ID: "question-http", SessionID: "session", ToolCallID: "tool", Questions: []question.Question{{ID: "q", Type: question.TypeFreeText, Text: "What should change?", Description: "Describe the required change."}}})
		answers <- a
	}()
	select {
	case <-events:
	case <-ctx.Done():
		t.Fatal("question was not published")
	}
	input := proto.QuestionAnswer{BatchRequestID: "question-http", Responses: []proto.QuestionResponse{{QuestionID: "q", FillInText: "The complete answer over HTTP"}}}
	submit := func(input proto.QuestionAnswer) (int, proto.QuestionAnswerResponse) {
		t.Helper()
		body, err := json.Marshal(input)
		require.NoError(t, err)
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, h.httpSrv.URL+"/v1/workspaces/"+h.workspace.ID+"/questions/answer", bytes.NewReader(body))
		require.NoError(t, err)
		r.Header.Set("Content-Type", "application/json")
		response, err := h.httpSrv.Client().Do(r)
		require.NoError(t, err)
		defer response.Body.Close()
		var ack proto.QuestionAnswerResponse
		if response.StatusCode == http.StatusOK {
			require.NoError(t, json.NewDecoder(response.Body).Decode(&ack))
		}
		return response.StatusCode, ack
	}
	status, ack := submit(input)
	require.Equal(t, http.StatusOK, status)
	require.True(t, ack.Resolved)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "successful HTTP acknowledgment requires persisted answer")
	select {
	case got := <-answers:
		require.Equal(t, input.Responses[0].FillInText, got[0].FillInText)
	case <-ctx.Done():
		t.Fatal("accepted answer did not reach the tool")
	}
	status, ack = submit(input)
	require.Equal(t, http.StatusOK, status)
	require.True(t, ack.Resolved, "a lost HTTP response can be safely retried")
	input.Responses[0].FillInText = "Do not silently replace the first answer"
	status, ack = submit(input)
	require.NotEqual(t, http.StatusOK, status)
	require.False(t, ack.Resolved)
}
