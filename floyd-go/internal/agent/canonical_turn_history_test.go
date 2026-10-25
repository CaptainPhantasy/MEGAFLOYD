package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

type canonicalHistoryModel struct {
	finishStreamModel
	calls int
	input string
}

func (m *canonicalHistoryModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	if m.calls != 1 {
		return m.finishStreamModel.Stream(ctx, call)
	}
	return func(yield func(fantasy.StreamPart) bool) {
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeToolCall, ID: "write-history", ToolCallName: tools.WriteToolName, ToolCallInput: m.input}) {
			return
		}
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls})
	}, nil
}

type canonicalHistoryPermission struct {
	permission.Service
	requests int
}

func (p *canonicalHistoryPermission) Request(ctx context.Context, request permission.CreatePermissionRequest) (bool, error) {
	p.requests++
	return p.Service.Request(ctx, request)
}

func TestCanonicalActualRunCapturesNativeWriteAndQueuedTurn(t *testing.T) {
	env := testEnv(t)
	sess, err := env.sessions.Create(t.Context(), "history integration")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "Existing verification session"}}})
	require.NoError(t, err)
	input, err := json.Marshal(tools.WriteParams{FilePath: "native.txt", Content: "canonical native write\n"})
	require.NoError(t, err)
	model := &canonicalHistoryModel{finishStreamModel: finishStreamModel{text: "done"}, input: string(input)}
	permissions := &canonicalHistoryPermission{Service: env.permissions}
	nativeWrite := tools.NewWriteTool(nil, permissions, env.history, *env.filetracker, env.workingDir)
	sa := testSessionAgent(env, model, model, "canonical history", nativeWrite).(*sessionAgent)
	sa.history = env.history
	sa.workingDir = env.workingDir
	recorder := env.history.(history.TurnRecorder)
	// Acceptance of a busy queued prompt must not create a fake snapshot or call a model.
	sa.activeRequests.Set(sess.ID, &activeCancel{cancel: func() {}})
	_, err = sa.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, RunID: "queued-history", Prompt: "queued no edit"})
	require.NoError(t, err)
	before, err := recorder.ListTurns(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Empty(t, before)
	require.Zero(t, model.calls)
	sa.activeRequests.Del(sess.ID)
	_, err = sa.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, RunID: "first-history", Prompt: "create tracked file"})
	require.NoError(t, err)
	require.Equal(t, 3, model.calls, "actual write loop has two steps and queued next turn has one")
	require.Equal(t, 1, permissions.requests, "native permission path must still run")
	data, err := os.ReadFile(filepath.Join(env.workingDir, "native.txt"))
	require.NoError(t, err)
	require.Equal(t, "canonical native write\n", string(data))
	turns, err := recorder.ListTurns(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	require.Equal(t, int64(1), turns[0].TurnID)
	require.Equal(t, int64(2), turns[1].TurnID)
	for i, want := range []string{"create tracked file", "queued no edit"} {
		require.True(t, turns[i].Recorded)
		user, err := env.messages.Get(t.Context(), turns[i].UserMessageID)
		require.NoError(t, err)
		require.Equal(t, message.User, user.Role)
		require.Equal(t, want, user.Content().Text)
	}
	changes, err := recorder.TurnChanges(t.Context(), sess.ID, 1, env.workingDir)
	require.NoError(t, err)
	require.Equal(t, []history.TurnChange{{Path: "native.txt", Status: "added", Additions: 1}}, changes.Changes)
	beforeContent, err := recorder.TurnContent(t.Context(), sess.ID, 1, "native.txt", "start", env.workingDir)
	require.NoError(t, err)
	require.Nil(t, beforeContent)
	afterContent, err := recorder.TurnContent(t.Context(), sess.ID, 1, "native.txt", "end", env.workingDir)
	require.NoError(t, err)
	require.NotNil(t, afterContent)
	require.Equal(t, string(data), *afterContent.Content)
	queuedChanges, err := recorder.TurnChanges(t.Context(), sess.ID, 2, env.workingDir)
	require.NoError(t, err)
	require.True(t, queuedChanges.Recorded)
	require.Empty(t, queuedChanges.Changes)

	coordinator := &coordinator{currentAgent: sa}
	accepted := sa.BeginAccepted(sess.ID)
	mutated := false
	require.ErrorIs(t, coordinator.WithSessionIdle(sess.ID, func() error { mutated = true; return nil }), ErrSessionBusy)
	require.False(t, mutated, "session operation must not cross an accepted-but-not-active run")
	accepted.Close()
	require.NoError(t, coordinator.WithSessionIdle(sess.ID, func() error { mutated = true; return nil }))
	require.True(t, mutated)
}
