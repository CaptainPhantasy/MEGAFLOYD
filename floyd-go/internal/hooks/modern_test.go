package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

func TestModernRequiredOrdersAndStops(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner([]config.HookConfig{
		{Command: `printf 'first\n' >> order; printf '{"updated_input":{"command":"new"}}'`, Required: true},
		{Command: `printf '%s\n' "$MEGAFLOYD_TOOL_INPUT_COMMAND" >> order; exit 7`, Required: true},
		{Command: `printf 'never\n' >> order`},
	}, dir, dir)
	result, err := r.Run(t.Context(), EventPreToolUse, "sid", "bash", `{"command":"old"}`)
	require.NoError(t, err)
	require.True(t, result.RequiredFailure)
	require.Equal(t, DecisionDeny, result.Decision)
	b, err := os.ReadFile(filepath.Join(dir, "order"))
	require.NoError(t, err)
	require.Equal(t, "first\nnew\n", string(b))
	require.Equal(t, 2, result.HookCount)
}

func TestModernRequiredOutputValidation(t *testing.T) {
	for _, out := range []string{`not json`, `{"version":2}`, `{"decision":"allow","decision":"deny"}`, `{"decision":"unknown"}`, `{"decision":"deny","hookSpecificOutput":{"permissionDecision":"allow"}}`, `{"updated_input":[]}`, `{"updated_input":"{\"x\":1,\"x\":2}"}`, `{"halt":"yes"}`, `{} {}`} {
		t.Run(out, func(t *testing.T) {
			r := NewRunner([]config.HookConfig{{Command: "printf '%s' '" + out + "'", Required: true}}, t.TempDir(), t.TempDir())
			got, err := r.Run(t.Context(), EventPreToolUse, "sid", "bash", `{}`)
			require.NoError(t, err)
			require.True(t, got.RequiredFailure)
		})
	}
	for _, out := range []string{"", `{"version":1}`, `{"decision":"none"}`, `{"hookSpecificOutput":{"permissionDecision":"allow"}}`} {
		require.NoError(t, validateRequiredOutput(out), out)
	}
}

func TestModernPostChecksCannotUndoOrGrant(t *testing.T) {
	r := NewRunner([]config.HookConfig{{Command: `printf '{"decision":"deny","halt":true,"updated_input":{"command":"bad"}}'`, Required: true}}, t.TempDir(), t.TempDir())
	got, err := r.Run(t.Context(), EventPostToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.True(t, got.RequiredFailure)
	require.Equal(t, DecisionNone, got.Decision)
	require.False(t, got.Halt)
	require.Empty(t, got.UpdatedInput)
	for _, event := range []string{EventPostToolUseFailure, EventActionRejected, EventToolCancelled, EventPermissionRequest, EventPermissionResolved, EventStop, EventSessionStart} {
		r := NewRunner([]config.HookConfig{{Command: `printf '{"decision":"deny","halt":true,"updated_input":{"command":"bad"}}'`}}, t.TempDir(), t.TempDir())
		got, err := r.Run(t.Context(), event, "sid", "bash", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionNone, got.Decision, event)
		require.False(t, got.Halt, event)
		require.Empty(t, got.UpdatedInput, event)
	}
}

func TestModernPreviewMatchesExecutionWithoutCommands(t *testing.T) {
	dir := t.TempDir()
	cmd := `printf secret-marker > ran`
	r := NewRunner([]config.HookConfig{
		{Command: cmd, Matcher: "bash", Required: true},
		{Command: cmd, Matcher: "bash", Required: true},
		{Command: cmd, Matcher: "edit"},
		{Command: cmd, Matcher: "["},
	}, dir, dir)
	plan := r.Preview(EventPreToolUse, "bash")
	data, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NotContains(t, string(data), "secret-marker")
	_, err = os.Stat(filepath.Join(dir, "ran"))
	require.True(t, os.IsNotExist(err))
	require.Equal(t, 1, plan[0].Order)
	require.Equal(t, "ordered", plan[0].Mode)
	require.NotNil(t, plan[1].DuplicateOf)
	require.Equal(t, 0, *plan[1].DuplicateOf)
	require.False(t, plan[2].Matched)
	require.NotEmpty(t, plan[3].Error)
	var mu sync.Mutex
	var records []Execution
	r.SetRecorder(func(e Execution) { mu.Lock(); records = append(records, e); mu.Unlock() })
	_, err = r.Run(t.Context(), EventPreToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, 0, records[0].ConfigIndex)
	require.Equal(t, records[0].ID, records[1].ID)
	require.Equal(t, "succeeded", records[1].State)
	b, err := json.Marshal(records)
	require.NoError(t, err)
	require.NotContains(t, string(b), "secret-marker")
}

func TestModernNativeDetailsAndPayload(t *testing.T) {
	dir := t.TempDir()
	exit := 3
	ctx := WithEventDetails(t.Context(), EventDetails{EventID: "event", Timestamp: "2026-10-25T00:00:00Z", ToolCallID: "call", RunID: "run", ParentToolCallID: "parent", Outcome: "failed", ExitCode: &exit, ToolResponse: json.RawMessage(`{"status":"actual"}`)})
	r := NewRunner([]config.HookConfig{{Command: `cat > payload`, Required: true}}, dir, dir)
	var records []Execution
	r.SetRecorder(func(e Execution) { records = append(records, e) })
	_, err := r.Run(ctx, EventPostToolUse, "sid", "bash", `{"event_id":"spoof","run_id":"spoof"}`)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, "payload"))
	require.NoError(t, err)
	require.NoError(t, ValidatePayload(b))
	var p Payload
	require.NoError(t, json.Unmarshal(b, &p))
	require.Equal(t, "event", p.EventID)
	require.Equal(t, "run", p.RunID)
	require.Equal(t, "call", records[0].ToolCallID)
	require.Equal(t, "bash", records[0].ToolName)
	require.Equal(t, "parent", records[0].ParentToolCallID)
	require.Error(t, ValidatePayload([]byte(`{"version":2}`)))
}

func TestModernAsyncTracksCompletionAndCancel(t *testing.T) {
	r := NewRunner([]config.HookConfig{{Command: `printf '{"decision":"deny","halt":true,"updated_input":{"x":1}}'`, Async: true}}, t.TempDir(), t.TempDir())
	got, err := r.Run(t.Context(), EventPostToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.Len(t, got.AsyncJobs, 1)
	require.Equal(t, DecisionNone, got.Decision)
	require.Empty(t, got.UpdatedInput)
	job, err := r.AwaitAsync(t.Context(), got.AsyncJobs[0])
	require.NoError(t, err)
	require.Equal(t, "denied", job.State)
	require.False(t, r.CancelAsync(job.ID))
	require.True(t, r.ForgetAsync(job.ID))
	require.Empty(t, r.AsyncResults())
	slow := NewRunner([]config.HookConfig{{Command: `sleep 20`, Async: true}}, t.TempDir(), t.TempDir())
	got, err = slow.Run(t.Context(), EventPostToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.True(t, slow.CancelAsync(got.AsyncJobs[0]))
	wait, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, slow.DrainAsync(wait))
	job, err = slow.AwaitAsync(wait, got.AsyncJobs[0])
	require.NoError(t, err)
	require.Equal(t, "cancelled", job.State)
}

func TestModernRequiredTimeoutAndCancel(t *testing.T) {
	r := NewRunner([]config.HookConfig{{Command: `sleep 20`, Timeout: 1, Required: true}}, t.TempDir(), t.TempDir())
	var records []Execution
	r.SetRecorder(func(e Execution) { records = append(records, e) })
	got, err := r.Run(t.Context(), EventPreToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.True(t, got.RequiredFailure)
	require.Equal(t, "timed_out", records[len(records)-1].State)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err = r.Run(ctx, EventPreToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.True(t, got.Cancelled)
	require.Equal(t, DecisionDeny, got.Decision)
}

func TestModernLiteralRewriteKeys(t *testing.T) {
	got, err := shallowMerge(`{"a":{"b":"unchanged"}}`, `{"a.b":"literal",":x":"value"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"a":{"b":"unchanged"},"a.b":"literal",":x":"value"}`, got)
}

func TestModernEventsAndEnv(t *testing.T) {
	for _, event := range EventNames() {
		s, ok := EventSemantics(event)
		require.True(t, ok)
		if event != EventPreToolUse {
			require.False(t, s.CanBlock)
		}
	}
	env := strings.Join(BuildEnv(EventPreToolUse, "bash", "sid", "/tmp", "/tmp", `{"command":"pwd"}`), "\n")
	require.Contains(t, env, "MEGAFLOYD_TOOL_INPUT_COMMAND=pwd")
	require.Contains(t, env, "FLOYD_TOOL_INPUT_COMMAND=pwd")
	require.Contains(t, env, "CRUSH_TOOL_INPUT_COMMAND=pwd")
}

func TestModernAsyncOwnerLifetimeSeparateFromSync(t *testing.T) {
	dir := t.TempDir()
	owner, stopOwner := context.WithCancel(t.Context())
	defer stopOwner()
	caller, cancelCaller := context.WithCancel(t.Context())
	ctx := WithAsyncContext(caller, owner)
	async := NewRunner([]config.HookConfig{{Command: `sleep 0.05; printf done > completed`, Async: true}}, dir, dir)
	got, err := async.Run(ctx, EventPostToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	cancelCaller()
	job, err := async.AwaitAsync(t.Context(), got.AsyncJobs[0])
	require.NoError(t, err)
	require.Equal(t, "succeeded", job.State)
	_, err = os.Stat(filepath.Join(dir, "completed"))
	require.NoError(t, err)
	syncRunner := NewRunner([]config.HookConfig{{Command: `printf never > unexpected`, Required: true}}, dir, dir)
	got, err = syncRunner.Run(ctx, EventPostToolUse, "sid", "bash", `{}`)
	require.NoError(t, err)
	require.True(t, got.Cancelled)
	require.True(t, got.RequiredFailure)
	_, err = os.Stat(filepath.Join(dir, "unexpected"))
	require.True(t, os.IsNotExist(err))
}
