package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/backend"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/legacy-ai/floyd/internal/question"
	"github.com/stretchr/testify/require"
)

type canonicalIntelligenceCoordinator struct {
	*stubCoordinator
	calls     chan [2]string
	refreshed chan struct{}
}

func (c *canonicalIntelligenceCoordinator) SuggestPrompt(_ context.Context, sessionID, prefix string) (string, error) {
	c.calls <- [2]string{sessionID, prefix}
	return prefix + " the gateway tools", nil
}

func (c *canonicalIntelligenceCoordinator) RefreshTools(context.Context) error {
	c.refreshed <- struct{}{}
	return nil
}

func (c *canonicalIntelligenceCoordinator) ToolCatalog() []fantasy.ToolInfo {
	return []fantasy.ToolInfo{{
		Name: "run_skill", Description: "Execute an installed skill",
		Parameters: map[string]any{"name": map[string]any{"type": "string"}},
		Required:   []string{"name"},
	}}
}

// The real HTTP router and backend delegate to the installed coordinator and
// replay interactions created before a new web/TUI client subscribed.
func TestCanonicalIntelligenceHTTPAndInteractionHandoff(t *testing.T) {
	h := newE2EHarness(t)
	coord := &canonicalIntelligenceCoordinator{stubCoordinator: &stubCoordinator{}, calls: make(chan [2]string, 1), refreshed: make(chan struct{}, 1)}
	h.app.AgentCoordinator = coord
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	base := "/v1/workspaces/" + h.workspace.ID

	request := func(method, path string, body any, result any) {
		t.Helper()
		var encoded []byte
		if body != nil {
			var err error
			encoded, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req, err := http.NewRequestWithContext(ctx, method, h.httpSrv.URL+base+path, bytes.NewReader(encoded))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.httpSrv.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(result))
	}

	var suggestion proto.SuggestPromptResponse
	request(http.MethodPost, "/agent/sessions/canonical-session/suggestion",
		proto.SuggestPromptRequest{Prefix: "Inspect"}, &suggestion)
	require.Equal(t, "Inspect the gateway tools", suggestion.Suggestion)
	require.Equal(t, [2]string{"canonical-session", "Inspect"}, <-coord.calls)

	var tools []backend.ToolDescriptor
	request(http.MethodGet, "/tools", nil, &tools)
	select {
	case <-coord.refreshed:
	default:
		t.Fatal("tool route did not refresh the installed palette")
	}
	require.Len(t, tools, 1)
	require.Equal(t, "run_skill", tools[0].Name)
	require.Equal(t, "object", tools[0].InputSchema["type"])
	require.Equal(t, []any{"name"}, tools[0].InputSchema["required"])
	require.Equal(t, map[string]any{"name": map[string]any{"type": "string"}}, tools[0].InputSchema["properties"])

	permissionDone := make(chan bool, 1)
	go func() {
		granted, _ := h.app.Permissions.Request(ctx, permission.CreatePermissionRequest{
			SessionID: "canonical-session", ToolCallID: "skill-call", ToolName: "run_skill",
			Action: "execute", Path: h.workspace.Path, Description: "Run a configured skill",
		})
		permissionDone <- granted
	}()
	questionDone := make(chan error, 1)
	go func() {
		_, err := h.app.Questions.Ask(ctx, question.Request{
			ID: "current-batch", SessionID: "canonical-session", ToolCallID: "question-call",
			Questions: []question.Question{{
				ID: "which-tool", Type: question.TypeFreeText, Text: "Which tool?",
				Description: "Select the next tool for this session",
			}},
		})
		questionDone <- err
	}()
	require.Eventually(t, func() bool {
		return len(h.app.Permissions.Pending()) == 1 && len(h.app.Questions.Pending()) == 1
	}, 2*time.Second, 5*time.Millisecond)

	var pending proto.PendingInteractions
	request(http.MethodGet, "/interactions", nil, &pending)
	require.Len(t, pending.Permissions, 1)
	require.Equal(t, "skill-call", pending.Permissions[0].ToolCallID)
	require.Len(t, pending.Questions, 1)
	require.Equal(t, "current-batch", pending.Questions[0].ID)

	// A new subscription must see the already-pending permission.
	events, stop := h.subscribeSSE(t, ctx, h.workspace.ID, uuid.NewString())
	defer stop()
	replayed, ok := drainUntil[pubsub.Event[proto.PermissionRequest]](ctx, events, func(ev pubsub.Event[proto.PermissionRequest]) bool {
		return ev.Payload.ID == pending.Permissions[0].ID
	})
	require.True(t, ok, "pending interaction was not replayed to the attaching client")
	require.Equal(t, "canonical-session", replayed.Payload.SessionID)

	var resolution proto.QuestionAnswerResponse
	request(http.MethodPost, "/questions/cancel", map[string]string{"batch_request_id": "previous-batch"}, &resolution)
	require.False(t, resolution.Resolved, "stale clients must not cancel a newer question")
	require.Len(t, h.app.Questions.Pending(), 1)
	request(http.MethodPost, "/questions/cancel", map[string]string{"batch_request_id": "current-batch"}, &resolution)
	require.True(t, resolution.Resolved)
	select {
	case err := <-questionDone:
		require.ErrorIs(t, err, question.ErrCancelled)
	case <-ctx.Done():
		t.Fatal("question cancellation did not unblock the asking agent")
	}

	require.True(t, h.app.Permissions.Deny(permission.PermissionRequest{
		ID: pending.Permissions[0].ID, ToolCallID: "skill-call",
	}))
	select {
	case granted := <-permissionDone:
		require.False(t, granted)
	case <-ctx.Done():
		t.Fatal("permission resolution did not unblock the waiting tool")
	}
	request(http.MethodGet, "/interactions", nil, &pending)
	require.Empty(t, pending.Permissions)
	require.Empty(t, pending.Questions)
}
