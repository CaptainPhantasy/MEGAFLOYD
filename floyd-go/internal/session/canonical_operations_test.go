package session_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

type canonicalSessionOperations interface {
	UpdateMetadata(context.Context, string, *string, map[string]any) (session.Session, error)
	SetArchived(context.Context, string, bool) (session.Session, error)
	Clone(context.Context, string, *string, map[string]any, bool) (session.Session, error)
	Undo(context.Context, string, int) (*session.UndoResult, error)
}

func TestCanonicalSessionOperationsAndTurnHistory(t *testing.T) {
	ctx := t.Context()
	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, ".megafloyd")
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dataDir)) })
	queries := db.New(conn)
	sessions := session.NewService(queries, conn)
	messages := message.NewService(queries)
	files := history.NewService(queries, conn)
	recorder := files.(history.TurnRecorder)
	ops := sessions.(canonicalSessionOperations)
	sess, err := sessions.Create(ctx, "canonical history")
	require.NoError(t, err)
	stale := sess
	_, err = ops.UpdateMetadata(ctx, sess.ID, nil, map[string]any{"cwd": workDir, "pinned": true, "settings": map[string]any{"profile": "review"}, "goal": map[string]any{"status": "active", "id": "source-goal"}})
	require.NoError(t, err)
	archived, err := ops.SetArchived(ctx, sess.ID, true)
	require.NoError(t, err)
	require.True(t, archived.Archived)
	require.NotNil(t, archived.ArchivedAt)
	stale.PromptTokens = 42
	_, err = sessions.Save(ctx, stale)
	require.NoError(t, err)
	sess, err = sessions.Get(ctx, sess.ID)
	require.NoError(t, err)
	require.True(t, sess.Archived, "ordinary usage save must not erase archive state")
	require.Equal(t, true, sess.Metadata["pinned"], "ordinary save must not erase metadata")
	sess, err = ops.SetArchived(ctx, sess.ID, false)
	require.NoError(t, err)
	require.False(t, sess.Archived)
	require.Nil(t, sess.ArchivedAt)

	addMessage := func(role message.MessageRole, text string) message.Message {
		msg, e := messages.Create(ctx, sess.ID, message.CreateMessageParams{Role: role, Parts: []message.ContentPart{message.TextContent{Text: text}}})
		require.NoError(t, e)
		return msg
	}
	path := filepath.Join(workDir, "example.txt")
	require.NoError(t, os.WriteFile(path, []byte("before\n"), 0600))
	first := addMessage(message.User, "edit file")
	turnCtx, err := recorder.BeginTurn(ctx, sess.ID, first.ID, workDir)
	require.NoError(t, err)
	require.NoError(t, history.CaptureBefore(turnCtx, files, path))
	require.NoError(t, os.WriteFile(path, []byte("intermediate\n"), 0600))
	require.NoError(t, history.CaptureBefore(turnCtx, files, path))
	require.NoError(t, os.WriteFile(path, []byte("after\nadded\n"), 0600))
	addMessage(message.Assistant, "edited")
	require.NoError(t, recorder.CompleteTurn(turnCtx))
	changes, err := recorder.TurnChanges(ctx, sess.ID, 0, workDir)
	require.NoError(t, err)
	require.True(t, changes.Recorded)
	require.Equal(t, []history.TurnChange{{Path: "example.txt", Status: "modified", Additions: 2, Deletions: 1}}, changes.Changes)
	start, err := recorder.TurnContent(ctx, sess.ID, 0, "example.txt", "start", workDir)
	require.NoError(t, err)
	require.Equal(t, "before\n", *start.Content)
	end, err := recorder.TurnContent(ctx, sess.ID, 0, "example.txt", "end", workDir)
	require.NoError(t, err)
	require.Equal(t, "after\nadded\n", *end.Content)

	second := addMessage(message.User, "no edits")
	turnCtx, err = recorder.BeginTurn(ctx, sess.ID, second.ID, workDir)
	require.NoError(t, err)
	addMessage(message.Assistant, "no file changes")
	require.NoError(t, recorder.CompleteTurn(turnCtx))
	empty, err := recorder.TurnChanges(ctx, sess.ID, 1, workDir)
	require.NoError(t, err)
	require.True(t, empty.Recorded, "completed zero-edit turns must remain distinguishable from old unrecorded turns")
	require.Empty(t, empty.Changes)
	missing, err := recorder.TurnChanges(ctx, sess.ID, 50, workDir)
	require.NoError(t, err)
	require.False(t, missing.Recorded)

	third := addMessage(message.User, "delete file")
	turnCtx, err = recorder.BeginTurn(ctx, sess.ID, third.ID, workDir)
	require.NoError(t, err)
	require.NoError(t, history.CaptureBefore(turnCtx, files, path))
	require.NoError(t, os.Remove(path))
	addMessage(message.Assistant, "deleted")
	require.NoError(t, recorder.CompleteTurn(turnCtx))
	turns, err := recorder.ListTurns(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, []history.Turn{{TurnID: 0, UserMessageID: first.ID, Recorded: true}, {TurnID: 1, UserMessageID: second.ID, Recorded: true}, {TurnID: 2, UserMessageID: third.ID, Recorded: true}}, turns)
	removed, err := recorder.TurnChanges(ctx, sess.ID, 2, workDir)
	require.NoError(t, err)
	require.Equal(t, "deleted", removed.Changes[0].Status)
	deletedContent, err := recorder.TurnContent(ctx, sess.ID, 2, "example.txt", "end", workDir)
	require.NoError(t, err)
	require.Nil(t, deletedContent)

	cloned, err := ops.Clone(ctx, sess.ID, nil, map[string]any{"custom": "kept"}, false)
	require.NoError(t, err)
	require.Equal(t, sess.ID, cloned.ParentSessionID)
	require.Equal(t, "kept", cloned.Metadata["custom"])
	require.Equal(t, true, cloned.Metadata["pinned"])
	require.NotContains(t, cloned.Metadata, "goal", "fork must not inherit active goal execution")
	sourceMessages, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	clonedMessages, err := messages.List(ctx, cloned.ID)
	require.NoError(t, err)
	require.Len(t, clonedMessages, len(sourceMessages)+1)
	for i, msg := range sourceMessages {
		require.NotEqual(t, msg.ID, clonedMessages[i].ID)
		require.Equal(t, msg.Role, clonedMessages[i].Role)
		require.Equal(t, msg.Parts, clonedMessages[i].Parts)
	}
	require.Contains(t, clonedMessages[len(sourceMessages)].Content().Text, "[Fork]")
	clonedTurns, err := recorder.ListTurns(ctx, cloned.ID)
	require.NoError(t, err)
	require.Len(t, clonedTurns, 3)
	require.Equal(t, clonedMessages[0].ID, clonedTurns[0].UserMessageID)
	require.NotEqual(t, first.ID, clonedTurns[0].UserMessageID)
	clonedStart, err := recorder.TurnContent(ctx, cloned.ID, 0, "example.txt", "start", workDir)
	require.NoError(t, err)
	require.Equal(t, "before\n", *clonedStart.Content)
	branch, err := ops.Clone(ctx, sess.ID, nil, map[string]any{"cwd": workDir}, true)
	require.NoError(t, err)
	branchMessages, err := messages.List(ctx, branch.ID)
	require.NoError(t, err)
	require.Empty(t, branchMessages)
	require.Equal(t, sess.ID, branch.ParentSessionID)
	listed, err := sessions.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 3, "user-created forks and child branches must be visible with their parent")

	_, err = ops.Undo(ctx, sess.ID, 99)
	require.ErrorIs(t, err, session.ErrInvalidUndo)
	remaining, err := messages.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, remaining, 6, "invalid undo must not mutate the transcript")
	undone, err := ops.Undo(ctx, sess.ID, 1)
	require.NoError(t, err)
	require.Equal(t, 1, undone.RemovedTurns)
	require.Len(t, undone.RemovedMessageIDs, 2)
	require.Equal(t, int64(42), undone.Session.PromptTokens, "undo must retain incurred usage")
	remaining, err = messages.List(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, remaining, 4)
	require.NoFileExists(t, path, "transcript undo must not silently modify files on disk")
	turns, err = recorder.ListTurns(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, turns, 2, "deleted user message must remove its active turn mapping")
	var payload string
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT payload FROM session_undo_records WHERE id = ?", undone.RecordID).Scan(&payload))
	var recovery struct {
		Messages []json.RawMessage `json:"messages"`
		Turns    []json.RawMessage `json:"turns"`
		Files    []json.RawMessage `json:"turn_files"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &recovery))
	require.Len(t, recovery.Messages, 2)
	require.Len(t, recovery.Turns, 1)
	require.Len(t, recovery.Files, 1)
	// Verify persistence using new service instances over the canonical database.
	reopened, err := session.NewService(queries, conn).Get(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, true, reopened.Metadata["pinned"])
	require.False(t, reopened.Archived)
}
