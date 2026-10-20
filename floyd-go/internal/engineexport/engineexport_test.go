package engineexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/message"
)

func TestExportWritesEngineSessionAndIsIdempotent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	created := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	messages := []Message{
		{Role: "user", Parts: []message.ContentPart{message.TextContent{Text: "hello"}}, CreatedAt: created},
		{Role: "assistant", Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "plan"},
			message.TextContent{Text: "hi"},
		}, CreatedAt: created.Add(time.Second)},
		{Role: "tool", Parts: []message.ContentPart{message.ToolResult{ToolCallID: "call_1", Content: "output"}}, CreatedAt: created.Add(2 * time.Second)},
	}

	result, err := Export(home, "abc-123", "Exported", "/tmp/project", messages)
	require.NoError(t, err)
	require.Equal(t, "session_abc-123", result.EngineID)
	require.Equal(t, 3, result.Messages)

	var state map[string]any
	data, err := os.ReadFile(filepath.Join(result.Dir, "state.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &state))
	require.Equal(t, "session_abc-123", state["id"])
	require.Equal(t, "Exported", state["title"])

	wire, err := os.ReadFile(filepath.Join(result.Dir, "agents", "main", "wire.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 3, strings.Count(string(wire), `"type":"agent.message.appended"`))
	require.Contains(t, string(wire), `"type":"context.append_message"`)
	require.Contains(t, string(wire), `"type":"content.part"`)
	require.Contains(t, string(wire), `"type":"tool.result"`)
	require.Contains(t, string(wire), `"type":"turn.prompt"`)
	require.Contains(t, string(wire), `"type":"prompt.completed"`)
	require.Contains(t, string(wire), `"role":"user"`)
	require.Contains(t, string(wire), `"type":"think"`)
	require.Contains(t, string(wire), `"toolCallId":"call_1"`)

	index, err := os.ReadFile(filepath.Join(home, "session_index.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(index), `"session_abc-123"`))

	_, err = Export(home, "abc-123", "Exported", "/tmp/project", messages)
	require.NoError(t, err)
	index, err = os.ReadFile(filepath.Join(home, "session_index.jsonl"))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(index), `"session_abc-123"`))
}
