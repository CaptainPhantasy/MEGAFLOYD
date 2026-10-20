package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/message"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		text    string
		isError bool
		ok      bool
		class   string
		code    int
		hasCode bool
	}{
		{"success", "all good", false, true, "", 0, false},
		{"not found", "/bin/bash: line 1: cargo: command not found", false, false, "not_found", 0, false},
		{"permission", "open /etc/x: permission denied", false, false, "permission", 0, false},
		{"timeout", "context deadline exceeded", false, false, "timeout", 0, false},
		{"exit code", "Command failed with exit code 2", false, false, "nonzero_exit", 2, true},
		{"tool error", "boost: unrecognized option", true, false, "tool_error", 0, false},
		{"build", "syntax error: unexpected EOF", false, false, "build", 0, false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ok, class, code, hasCode := Classify(test.text, test.isError)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.class, class)
			require.Equal(t, test.code, code)
			require.Equal(t, test.hasCode, hasCode)
		})
	}
}

func TestFromCLIWritesCallsResultsAndRetrySignal(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC).Unix()
	call := message.ToolCall{ID: "call_1", Name: "bash", Input: `{"command":"go test"}`}
	msgs := []message.Message{
		{ID: "m1", Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "run the tests"}}, CreatedAt: created},
		{ID: "m2", Role: message.Assistant, Parts: []message.ContentPart{call, message.TextContent{Text: "running"}}, CreatedAt: created + 1},
		{ID: "m3", Role: message.Tool, Parts: []message.ContentPart{message.ToolResult{
			ToolCallID: "call_1", Name: "bash", Content: "Command failed with exit code 1", IsError: true,
		}}, CreatedAt: created + 2},
	}

	events := FromCLI("session_test", msgs)
	require.Len(t, events, 4)

	require.Equal(t, KindPrompt, events[0].Kind)
	require.Equal(t, "1", events[0].Turn)

	var callEvent, resultEvent *Event
	for i := range events {
		switch events[i].Kind {
		case KindToolCall:
			callEvent = &events[i]
		case KindToolResult:
			resultEvent = &events[i]
		}
	}
	require.NotNil(t, callEvent)
	require.NotNil(t, resultEvent)
	require.Equal(t, "bash", callEvent.Tool)
	require.Equal(t, 1, callEvent.Attempt)
	require.NotNil(t, resultEvent.OK)
	require.False(t, *resultEvent.OK)
	require.NotNil(t, resultEvent.ExitCode)
	require.Equal(t, 1, *resultEvent.ExitCode)
	require.Equal(t, "nonzero_exit", resultEvent.ErrorClass)
}

func TestFromEngineReadsWireLog(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	agentDir := filepath.Join(home, "sessions", "wd_test", "session_abc", "agents", "main")
	require.NoError(t, os.MkdirAll(agentDir, 0o700))
	wire := `{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"fix it"}]}}
{"type":"context.append_loop_event","event":{"type":"tool.call","turnId":1,"step":1,"name":"Bash","toolCallId":"call_9","args":{"command":"make"},"time":1792470000000}}
{"type":"context.append_loop_event","event":{"type":"tool.result","toolCallId":"call_9","result":{"output":"make: command not found","isError":true},"time":1792470001000}}
{"type":"turn.ended","turnId":1,"reason":"cancelled","durationMs":1200,"time":1792470002000}
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "wire.jsonl"), []byte(wire), 0o600))

	events, err := FromEngine(home)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, KindPrompt, events[0].Kind)

	result := events[2]
	require.Equal(t, KindToolResult, result.Kind)
	require.NotNil(t, result.OK)
	require.False(t, *result.OK)
	require.Equal(t, "not_found", result.ErrorClass)

	require.Equal(t, KindTurnEnd, events[3].Kind)
	require.Equal(t, "cancelled", events[3].Signals["reason"])
	require.EqualValues(t, 1200, events[3].DurationMS)
}

func TestWriteSkipsDuplicateEvents(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	events := []Event{
		{ID: "a", Kind: KindPrompt, Session: "s", Source: "cli"},
		{ID: "b", Kind: KindAssistant, Session: "s", Source: "cli"},
	}
	written, err := Write(path, events)
	require.NoError(t, err)
	require.Equal(t, 2, written)

	written, err = Write(path, events)
	require.NoError(t, err)
	require.Equal(t, 0, written)

	loaded, err := Load(path)
	require.NoError(t, err)
	require.Len(t, loaded, 2)
}

func TestBuildOTLPGroupsSessionAndMarksFailure(t *testing.T) {
	t.Parallel()
	failed := false
	events := []Event{
		{ID: "1", Session: "session_a", Source: "cli", Kind: KindToolCall, Tool: "bash", CallID: "c1", TS: "2026-10-20T12:00:00Z"},
		{ID: "2", Session: "session_a", Source: "cli", Kind: KindToolResult, Tool: "bash", CallID: "c1", OK: &failed, ErrorClass: "tool_error", TS: "2026-10-20T12:00:01Z"},
	}
	request := BuildOTLP(events)
	require.Len(t, request.ResourceSpans, 1)
	spans := request.ResourceSpans[0].ScopeSpans[0].Spans
	require.Len(t, spans, 3)
	require.Equal(t, spans[0].SpanID, spans[1].ParentSpanID)
	require.Equal(t, spans[1].SpanID, spans[2].ParentSpanID)
	require.Equal(t, 2, spans[2].Status.Code)
}
