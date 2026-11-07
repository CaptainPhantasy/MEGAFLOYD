package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestQuestionCancelCarriesDisplayedRequestID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/workspaces/workspace/questions/cancel", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "displayed-question", body["batch_request_id"])
		_ = json.NewEncoder(w).Encode(proto.QuestionAnswerResponse{Resolved: true})
	}))
	defer srv.Close()
	c, err := NewClient("", "tcp", strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)
	accepted, err := c.CancelQuestionRequest(t.Context(), "workspace", "displayed-question")
	require.NoError(t, err)
	require.True(t, accepted)
}

func TestQuestionDeliveryPreservesServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(proto.Error{Message: "save question answer: disk is full"})
	}))
	defer srv.Close()
	c, err := NewClient("", "tcp", strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)
	accepted, err := c.AnswerQuestionBatch(t.Context(), "workspace", proto.QuestionAnswer{BatchRequestID: "q"})
	require.False(t, accepted)
	require.ErrorContains(t, err, "disk is full")
}
