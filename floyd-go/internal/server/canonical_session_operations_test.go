package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestCanonicalSessionControlsAndHistoryHTTP(t *testing.T) {
	h := newE2EHarness(t)
	dataDir := filepath.Join(h.workspace.Path, ".megafloyd")
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dataDir)) })
	q := db.New(conn)
	h.app.Sessions = session.NewService(q, conn)
	h.app.Messages = message.NewService(q)
	h.app.History = history.NewService(q, conn)
	coord := &stubCoordinator{busy: make(map[string]bool)}
	h.app.AgentCoordinator = coord
	sess, err := h.app.Sessions.Create(t.Context(), "shared controls")
	require.NoError(t, err)
	user, err := h.app.Messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Create file"}}})
	require.NoError(t, err)
	recorder := h.app.History.(history.TurnRecorder)
	turnCtx, err := recorder.BeginTurn(t.Context(), sess.ID, user.ID, h.workspace.Path)
	require.NoError(t, err)
	path := filepath.Join(h.workspace.Path, "http.txt")
	require.NoError(t, history.CaptureBefore(turnCtx, h.app.History, path))
	require.NoError(t, os.WriteFile(path, []byte("HTTP snapshot\n"), 0600))
	require.NoError(t, recorder.CompleteTurn(turnCtx))
	_, err = h.app.Messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "Created"}}})
	require.NoError(t, err)
	base := h.httpSrv.URL + "/v1/workspaces/" + h.workspace.ID + "/sessions/" + sess.ID
	request := func(method, suffix string, body, result any, status int) {
		t.Helper()
		var encoded []byte
		if body != nil {
			encoded, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, base+suffix, bytes.NewReader(encoded))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		response, err := h.httpSrv.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, status, response.StatusCode)
		if result != nil {
			// Every HTTP response is a fresh value; omitted fields must not retain the prior response.
			reflect.ValueOf(result).Elem().SetZero()
			require.NoError(t, json.NewDecoder(response.Body).Decode(result))
		}
	}
	var sessionResponse proto.Session
	request("POST", "/metadata", map[string]any{"title": "shared renamed", "metadata": map[string]any{"pinned": true, "cwd": h.workspace.Path}}, &sessionResponse, 200)
	require.Equal(t, "shared renamed", sessionResponse.Title)
	require.Equal(t, true, sessionResponse.Metadata["pinned"])
	request("POST", "/archive", map[string]bool{"archived": true}, &sessionResponse, 200)
	require.True(t, sessionResponse.Archived)
	request("POST", "/archive", map[string]bool{"archived": false}, &sessionResponse, 200)
	require.False(t, sessionResponse.Archived)
	require.Nil(t, sessionResponse.ArchivedAt)
	var settings map[string]any
	request("POST", "/settings", map[string]any{"system_prompt": "only this session", "tools": []string{}, "mcp_servers": []string{"gateway"}}, &settings, 200)
	require.Equal(t, []any{}, settings["tools"])
	request("POST", "/settings", map[string]any{"tools": nil, "profile": "review"}, &settings, 200)
	require.NotContains(t, settings, "tools")
	require.Equal(t, "only this session", settings["system_prompt"])
	request("GET", "/settings", nil, &settings, 200)
	require.Equal(t, "review", settings["profile"])
	request("POST", "/settings", map[string]any{"provider": "must-not-change-model"}, nil, 400)
	persisted, err := h.app.Sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Equal(t, true, persisted.Metadata["pinned"])
	require.Contains(t, persisted.Metadata, "agent_config")
	coord.busy[sess.ID] = true
	request("POST", "/undo", map[string]int{"count": 1}, nil, http.StatusConflict)
	coord.busy[sess.ID] = false
	request("POST", "/fork", map[string]string{"title": "copy"}, &sessionResponse, 200)
	require.Equal(t, sess.ID, sessionResponse.ParentSessionID)
	require.Equal(t, int64(3), sessionResponse.MessageCount)
	request("POST", "/branch", map[string]string{"title": "child"}, &sessionResponse, 200)
	require.Equal(t, sess.ID, sessionResponse.ParentSessionID)
	require.Zero(t, sessionResponse.MessageCount)
	var turns struct {
		Turns []history.Turn `json:"turns"`
	}
	request("GET", "/file-history/turns", nil, &turns, 200)
	require.Equal(t, []history.Turn{{TurnID: 0, UserMessageID: user.ID, Recorded: true}}, turns.Turns)
	var changes history.TurnChanges
	request("GET", "/file-history/changes?turn_id=0", nil, &changes, 200)
	require.True(t, changes.Recorded)
	require.Equal(t, []history.TurnChange{{Path: "http.txt", Status: "added", Additions: 1}}, changes.Changes)
	var content struct {
		Content *history.TurnContent `json:"content"`
	}
	request("GET", "/file-history/content?turn_id=0&path=http.txt&phase=end", nil, &content, 200)
	require.Equal(t, "HTTP snapshot\n", *content.Content.Content)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	deletedEvents := h.app.Messages.Subscribe(ctx)
	var undone proto.SessionUndoResponse
	request("POST", "/undo", map[string]int{"count": 1}, &undone, 200)
	require.Equal(t, 1, undone.RemovedTurns)
	require.NotEmpty(t, undone.UndoRecordID)
	require.Zero(t, undone.Session.MessageCount)
	deleted := 0
	for deleted < 2 {
		select {
		case event := <-deletedEvents:
			if event.Type == pubsub.DeletedEvent {
				deleted++
			}
		case <-ctx.Done():
			t.Fatal("attached clients did not receive removed transcript rows")
		}
	}
	require.FileExists(t, path)
}
