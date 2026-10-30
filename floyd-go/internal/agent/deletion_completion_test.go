package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/stretchr/testify/require"
)

func TestDeletionCompletionRequiresProof(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		err            error
		want           bool
	}{
		{name: "completed", metadata: `{"exit_code":0}`, want: true},
		{name: "nonzero", metadata: `{"exit_code":2}`},
		{name: "background", metadata: `{"background":true,"shell_id":"job"}`},
		{name: "cancelled", metadata: `{"exit_code":0}`, err: context.Canceled},
		{name: "timeout", metadata: `{"exit_code":0}`, err: context.DeadlineExceeded},
		{name: "missing"},
		{name: "malformed", metadata: `{`},
		{name: "null", metadata: `null`},
		{name: "nested failure", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, deletionSucceeded(fantasy.ToolResponse{Metadata: tc.metadata}, tc.err))
		})
	}
}

func TestDeletionCompletionParallelToolFormats(t *testing.T) {
	commands := []string{"rm a", "rm b"}
	for _, tc := range []struct {
		name, metadata string
		successes      []int
		finished       bool
	}{
		{name: "all successful", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":0}]}`, successes: []int{0, 1}, finished: true},
		{name: "individual failure", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":7}]}`, successes: []int{0}, finished: true},
		{name: "nested error", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":0,"error":"canceled"}]}`, successes: []int{0}, finished: false},
		{name: "missing exit code", metadata: `{"results":[{"index":0,"command":"rm a"},{"index":1,"command":"rm b","exit_code":0}]}`},
		{name: "missing result", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0}]}`},
		{name: "duplicate index", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":0,"command":"rm a","exit_code":0}]}`},
		{name: "wrong command", metadata: `{"results":[{"index":0,"command":"rm other","exit_code":0},{"index":1,"command":"rm b","exit_code":0}]}`},
		{name: "missing index", metadata: `{"results":[{"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":0}]}`},
		{name: "null", metadata: `null`},
		{name: "malformed", metadata: `{"results":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			successes, finished := deletionCompletion("parallel_bash", commands, fantasy.ToolResponse{Metadata: tc.metadata}, nil)
			require.Equal(t, tc.finished, finished)
			require.Equal(t, tc.successes, successes)
		})
	}
}

func TestDeletionCompletionActualBashMetadata(t *testing.T) {
	zero := 0
	response := fantasy.WithResponseMetadata(fantasy.NewTextResponse("done"), tools.BashResponseMetadata{ExitCode: &zero, Background: true})
	require.True(t, deletionSucceeded(response, nil), "a fast completed background call carries an exit code")
	response = fantasy.WithResponseMetadata(fantasy.NewTextResponse("running"), tools.BashResponseMetadata{ShellID: "job", Background: true})
	require.False(t, deletionSucceeded(response, nil))
	response = fantasy.ToolResponse{Metadata: `{"exit_code":0,"shell_id":"still-running"}`}
	require.False(t, deletionSucceeded(response, nil))
	response.IsError = true
	require.False(t, deletionSucceeded(response, nil))
}

func TestDeletionCompletionRecordingThroughGuard(t *testing.T) {
	for _, tc := range []struct {
		name, tool, metadata string
		err                  error
		successful           int
	}{
		{name: "bash success", tool: "bash", metadata: `{"exit_code":0}`, successful: 1},
		{name: "bash failure", tool: "bash", metadata: `{"exit_code":9}`},
		{name: "background", tool: "bash", metadata: `{"background":true,"shell_id":"job"}`},
		{name: "cancelled", tool: "bash", metadata: `{"exit_code":0}`, err: context.Canceled},
		{name: "timed out", tool: "bash", metadata: `{"exit_code":0}`, err: context.DeadlineExceeded},
		{name: "missing", tool: "bash"},
		{name: "malformed", tool: "bash", metadata: `{`},
		{name: "parallel mixed", tool: "parallel_bash", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":9}]}`, successful: 1},
		{name: "parallel success", tool: "parallel_bash", metadata: `{"results":[{"index":0,"command":"rm a","exit_code":0},{"index":1,"command":"rm b","exit_code":0}]}`, successful: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeletionFixture(t)
			f.answers = []bool{true, true}
			ran := false
			expected := fantasy.ToolResponse{Content: "original tool output", Metadata: tc.metadata}
			inner := deletionRecordingTool{ran: &ran, resp: expected, err: tc.err}
			wrapper := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
			input := `{"command":"rm a"}`
			if tc.tool == "parallel_bash" {
				input = `{"commands":[{"command":"rm a"},{"command":"rm b"}]}`
			}
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "completion-session")
			got, err := wrapper.Run(ctx, fantasy.ToolCall{ID: "completion-call", Name: tc.tool, Input: input})
			require.Equal(t, tc.err, err)
			require.True(t, ran)
			require.Equal(t, expected, got, "preserve original tool output and metadata")
			record := f.record("completion-session")
			var completed []string
			for _, execution := range record.Executions {
				completed = append(completed, execution.Targets...)
			}
			require.Len(t, completed, tc.successful)
		})
	}
}

func TestDeletionParallelOverlapDeniedBeforeExecution(t *testing.T) {
	for _, commands := range [][]string{{"rm a", "rm a"}, {"rm -rf a", "rm a/child"}, {"rm a/child", "rm -rf a"}} {
		f := newDeletionFixture(t)
		f.answers = []bool{true, true}
		ran := false
		inner := deletionRecordingTool{ran: &ran}
		wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
		params := tools.ParallelBashParams{}
		for _, command := range commands {
			params.Commands = append(params.Commands, tools.ParallelBashCommand{Command: command})
		}
		input, err := json.Marshal(params)
		require.NoError(t, err)
		response, err := wrapped.Run(context.WithValue(t.Context(), tools.SessionIDContextKey, "overlap"), fantasy.ToolCall{ID: "overlap", Name: tools.ParallelBashToolName, Input: string(input)})
		require.NoError(t, err)
		require.True(t, response.IsError)
		require.False(t, ran)
		require.Empty(t, f.asked)
	}
	require.False(t, overlappingDeletionCommands([]string{"rm a", "rm ab"}, t.TempDir()))
}

type completionMutatingTool struct {
	deletionRecordingTool
	after func() error
}

func (tool completionMutatingTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	if err := tool.after(); err != nil {
		return fantasy.ToolResponse{}, err
	}
	return tool.deletionRecordingTool.Run(ctx, call)
}

func TestDeletionCompletionUsesPreExecutionClassification(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	target := f.target("content")
	require.NoError(t, os.WriteFile(target, []byte("original fixture content"), 0o600))
	ran := false
	inner := completionMutatingTool{
		deletionRecordingTool: deletionRecordingTool{ran: &ran, resp: fantasy.ToolResponse{Metadata: `{"exit_code":0}`}},
		after:                 func() error { return os.WriteFile(target, nil, 0o600) },
	}
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
	response, err := wrapped.Run(context.WithValue(t.Context(), tools.SessionIDContextKey, "truncation"), fantasy.ToolCall{ID: "truncate", Name: tools.BashToolName, Input: `{"command":"> content"}`})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.True(t, ran)
	record := f.record("truncation")
	require.Len(t, record.Executions, 1, "the effect must not be reclassified after the file becomes empty")
	require.Equal(t, []string{target}, record.Executions[0].Targets)
}
