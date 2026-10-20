package enginebridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/message"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestListAndReadTranscript(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	first := filepath.Join(home, "sessions", "wd_a", "session_aaa")
	second := filepath.Join(home, "sessions", "wd_b", "session_bbb")
	writeFile(t, filepath.Join(first, "state.json"), `{"title":"Older","cwd":"/tmp/a","updatedAt":1000}`)
	writeFile(t, filepath.Join(second, "state.json"), `{"title":"Newer","cwd":"/tmp/b","updatedAt":2000}`)
	writeFile(t, filepath.Join(home, "session_index.jsonl"),
		`{"sessionId":"session_aaa","sessionDir":"`+first+`","workDir":"/tmp/a"}`+"\n"+
			`{"sessionId":"session_aaa","sessionDir":"`+first+`","workDir":"/tmp/a"}`+"\n"+
			`{"sessionId":"session_bbb","sessionDir":"`+second+`","workDir":"/tmp/b"}`+"\n"+
			"not-json\n")

	writeFile(t, filepath.Join(first, "agents", "main", "wire.jsonl"),
		`{"type":"metadata","protocol_version":"1.5"}`+"\n"+
			`{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"hello"}]},"meta":{"createdAt":"2026-10-01T00:00:00Z"}}}`+"\n"+
			`{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"think","think":"plan"},{"type":"text","text":"working"}]},"meta":{"createdAt":"2026-10-01T00:00:01Z"}}}`+"\n"+
			`{"type":"agent.message.appended","message":{"message":{"role":"tool","content":[{"type":"text","text":"result text"}],"toolCallId":"call_1"},"meta":{"createdAt":"2026-10-01T00:00:02Z"}}}`+"\n"+
			"{broken\n")

	sessions, err := List(home)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "session_bbb", sessions[0].ID)
	require.Equal(t, "Newer", sessions[0].Title)
	require.Equal(t, "session_aaa", sessions[1].ID)

	found, err := Find(home, "session_aaa")
	require.NoError(t, err)
	require.Equal(t, "Older", found.Title)

	transcript, err := ReadTranscript(found.Dir)
	require.NoError(t, err)
	require.Len(t, transcript, 3)

	require.Equal(t, "user", transcript[0].Role)
	require.Len(t, transcript[0].Parts, 1)
	require.Equal(t, "hello", transcript[0].Parts[0].(message.TextContent).Text)

	require.Equal(t, "assistant", transcript[1].Role)
	require.Len(t, transcript[1].Parts, 2)
	_, isReasoning := transcript[1].Parts[0].(message.ReasoningContent)
	require.True(t, isReasoning)
	require.Equal(t, "working", transcript[1].Parts[1].(message.TextContent).Text)

	require.Equal(t, "tool", transcript[2].Role)
	require.Len(t, transcript[2].Parts, 1)
	result, ok := transcript[2].Parts[0].(message.ToolResult)
	require.True(t, ok)
	require.Equal(t, "call_1", result.ToolCallID)
	require.Equal(t, "result text", result.Content)
}

func TestFindMissing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "session_index.jsonl"), "")
	_, err := Find(home, "session_nope")
	require.Error(t, err)
}
