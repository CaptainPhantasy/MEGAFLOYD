package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompleteUsesValidHeadersAndVersionedEndpoint(t *testing.T) {
	requests := make(chan CompletionRequest, 2)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "fixture-token" || r.Header.Get("Legacy-AI-Version") != "2023-06-01" {
			http.Error(w, "incorrect API contract", 400)
			return
		}
		var request CompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"reply-1","model":"fixture-model","usage":{"input_tokens":3,"output_tokens":2},"content":[{"type":"text","text":"Hello from fixture"}]}`))
	}))
	defer fixture.Close()
	for _, base := range []string{fixture.URL, fixture.URL + "/v1/"} {
		client := NewClient(ClientConfig{APIBaseURL: base, DefaultModel: "fixture-model", Retry: RetryOptions{MaxRetries: -1}}, "fixture-token")
		response, err := client.Complete("hello", CompletionOptions{Temperature: floatPtr(0), MaxTokens: intPtr(12), System: "fixture system"})
		require.NoError(t, err)
		require.Equal(t, "Hello from fixture", response.Content[0].Text)
		request := <-requests
		require.Equal(t, "fixture-model", request.Model)
		require.Equal(t, []Message{{Role: RoleUser, Content: "hello"}}, request.Messages)
		require.Equal(t, 0.0, *request.Temperature)
		require.Equal(t, 12, *request.MaxTokens)
		require.Equal(t, "fixture system", request.System)
	}
}

func TestCompleteStreamDeliversEventsAndReportsHTTPFailure(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "denied" {
			http.Error(w, "access denied", 403)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\ndata: [DONE]\n\n"))
	}))
	defer fixture.Close()
	client := NewClient(ClientConfig{APIBaseURL: fixture.URL}, "fixture")
	var events []StreamEvent
	require.NoError(t, client.CompleteStream([]Message{{Role: RoleUser, Content: "hello"}}, CompletionOptions{}, func(event StreamEvent) { events = append(events, event) }))
	require.Len(t, events, 1)
	require.Equal(t, "hello", events[0].Delta["text"])
	client = NewClient(ClientConfig{APIBaseURL: fixture.URL}, "denied")
	require.ErrorContains(t, client.CompleteStream(nil, CompletionOptions{}, func(StreamEvent) { t.Error("unexpected event") }), "access denied")
}

func TestPromptTemplatesPreserveLiteralValuesAndDefaults(t *testing.T) {
	t.Parallel()
	prompt, system, err := UsePromptTemplate("refactorCode", map[string]any{"code": "const value = '{focus}';", "focus": "speed"})
	require.NoError(t, err)
	require.Contains(t, prompt, "improve its speed")
	require.Contains(t, prompt, "const value = '{focus}';")
	require.Contains(t, prompt, "Additional context: None")
	require.NotEmpty(t, system)
	_, _, err = UsePromptTemplate("missing", nil)
	require.Error(t, err)
	require.Equal(t, []Message{{Role: RoleSystem, Content: "rules"}, {Role: RoleUser, Content: "question"}}, CreateConversation("question", "rules"))
	require.Equal(t, "File: main.go\n\n```go\npackage main\n```", CreateFileContextMessage("main.go", "package main", ""))
}

func TestBuildURLPreservesVersionPrefixAndQuery(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ base, want string }{
		{"https://example.test", "https://example.test/v1/messages"},
		{"https://example.test/v1/", "https://example.test/v1/messages"},
		{"https://example.test/api/v4", "https://example.test/api/v4/messages"},
		{"https://example.test/v1?route=fixture", "https://example.test/v1/messages?route=fixture"},
		{"https://v1", "https://v1/v1/messages"},
		{"https://example.test/tenant%2Fname/v1", "https://example.test/tenant%2Fname/v1/messages"},
		{"https://example.test/v1?route=fixture/", "https://example.test/v1/messages?route=fixture/"},
	} {
		t.Run(tc.base, func(t *testing.T) { require.Equal(t, tc.want, buildURL(tc.base, "/v1/messages")) })
	}
}
