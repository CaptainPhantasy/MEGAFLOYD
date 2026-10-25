// Package hookruntime connects configured hooks to native application events.
package hookruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/tidwall/sjson"
)

type (
	runKey    struct{}
	parentKey struct{}
	actionKey struct{}
)

type actionState struct {
	mu       sync.Mutex
	entered  bool
	rejected bool
	input    string
}

// WithRun preserves the accepted run ID without changing execution behavior.
func WithRun(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, runKey{}, id)
}

// WithParent marks the actual tool call that launched a native child agent.
func WithParent(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, parentKey{}, id)
}

func details(ctx context.Context) hooks.EventDetails {
	run, _ := ctx.Value(runKey{}).(string)
	parent, _ := ctx.Value(parentKey{}).(string)
	return hooks.EventDetails{EventID: uuid.NewString(), Timestamp: time.Now().UTC().Format(time.RFC3339Nano), RunID: run, ParentToolCallID: parent}
}

func mark(ctx context.Context, entered, rejected bool) {
	state, _ := ctx.Value(actionKey{}).(*actionState)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.entered = state.entered || entered
	state.rejected = state.rejected || rejected
	state.mu.Unlock()
}

// EnterTool distinguishes a tool body from an outer plan/scope rejection.
func EnterTool(ctx context.Context) { mark(ctx, true, false) }

// RejectTool records a native rejection without interpreting response prose.
func RejectTool(ctx context.Context) { mark(ctx, false, true) }

type cachedRunner struct {
	hash   string
	runner *hooks.Runner
}

type Runtime struct {
	config   *config.ConfigStore
	store    *hookstore.Store
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	current  map[string]cachedRunner
	runners  []*hooks.Runner
	closed   bool
	inflight sync.WaitGroup
}

func New(ctx context.Context, cfg *config.ConfigStore, store *hookstore.Store) *Runtime {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &Runtime{config: cfg, store: store, ctx: ctx, cancel: cancel, current: map[string]cachedRunner{}}
}

func (r *Runtime) Store() *hookstore.Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *Runtime) runner(event string) *hooks.Runner {
	if r == nil || r.config == nil {
		return nil
	}
	configured := r.config.Config().Hooks[event]
	if len(configured) == 0 {
		return nil
	}
	data, _ := json.Marshal(configured)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	if cached, ok := r.current[event]; ok && cached.hash == hash {
		return cached.runner
	}
	runner := hooks.NewRunner(configured, r.config.WorkingDir(), r.config.WorkingDir())
	runner.SetRecorder(r.record)
	r.current[event] = cachedRunner{hash: hash, runner: runner}
	r.runners = append(r.runners, runner)
	return runner
}

func (r *Runtime) record(execution hooks.Execution) {
	if r.store == nil {
		return
	}
	record := hookstore.Record{
		ID: execution.ID, EventID: execution.EventID, JobID: execution.AsyncJobID,
		Event: execution.Event, SessionID: execution.SessionID, ToolCallID: execution.ToolCallID,
		RunID: execution.RunID, ParentToolCallID: execution.ParentToolCallID, ToolName: execution.ToolName,
		HookName: execution.HookName, State: execution.State, Decision: execution.Decision, Outcome: execution.Outcome,
		StartedAt: execution.StartedAt, FinishedAt: execution.FinishedAt, ExitCode: execution.ExitCode,
		ConfigIndex: execution.ConfigIndex, CommandHash: execution.CommandHash, Mode: execution.Mode,
		ErrorKind: execution.ErrorKind, Required: execution.Required, Rewritten: execution.Rewritten,
	}
	if record.FinishedAt >= record.StartedAt && record.FinishedAt != 0 {
		record.DurationMS = (record.FinishedAt - record.StartedAt) / int64(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.store.Append(ctx, record); err != nil {
		slog.Error("Failed to retain hook execution metadata", "event", execution.Event, "execution_id", execution.ID)
	}
}

// Run uses the current configuration while retaining old async jobs until
// shutdown. Observer results cannot grant permissions or rewrite actions.
func (r *Runtime) Run(ctx context.Context, event, sid, name, input string, info hooks.EventDetails) (hooks.AggregateResult, error) {
	if r == nil {
		return hooks.AggregateResult{}, nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return hooks.AggregateResult{}, nil
	}
	r.inflight.Add(1)
	r.mu.Unlock()
	defer r.inflight.Done()
	runner := r.runner(event)
	if runner == nil {
		return hooks.AggregateResult{}, nil
	}
	base := details(ctx)
	if info.EventID == "" {
		info.EventID = base.EventID
	}
	if info.Timestamp == "" {
		info.Timestamp = base.Timestamp
	}
	if info.RunID == "" {
		info.RunID = base.RunID
	}
	if info.ParentToolCallID == "" {
		info.ParentToolCallID = base.ParentToolCallID
	}
	if event != hooks.EventPreToolUse && event != hooks.EventPostToolUse && event != hooks.EventSessionStart {
		ctx = context.WithoutCancel(ctx)
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	// Async observer jobs belong to the application, not a returning tool call.
	// Their own configured timeouts and Runtime.Close bound their lifetime.
	asyncCtx := hooks.WithEventDetails(context.WithoutCancel(ctx), info)
	asyncCtx, asyncCancel := context.WithCancel(asyncCtx)
	stopAsync := context.AfterFunc(r.ctx, asyncCancel)
	result, err := runner.Run(hooks.WithAsyncContext(hooks.WithEventDetails(ctx, info), asyncCtx), event, sid, name, input)
	stop()
	cancel()
	if len(result.AsyncJobs) == 0 {
		stopAsync()
		asyncCancel()
	} else {
		go func() {
			defer stopAsync()
			defer asyncCancel()
			for _, id := range result.AsyncJobs {
				_, _ = runner.AwaitAsync(asyncCtx, id)
				runner.ForgetAsync(id)
			}
		}()
	}
	return result, err
}

// Before evaluates the pre-action handlers and retains normal permission flow
// unless a configured synchronous pre handler explicitly returns allow.
func (r *Runtime) Before(ctx context.Context, sid string, call fantasy.ToolCall) (context.Context, fantasy.ToolCall, hooks.AggregateResult, bool) {
	info := details(ctx)
	info.ToolCallID, info.Outcome = call.ID, "requested"
	result, err := r.Run(ctx, hooks.EventPreToolUse, sid, call.Name, call.Input, info)
	if err != nil {
		slog.Warn("PreToolUse hook dispatch failed", "tool", call.Name)
	}
	if result.Decision == hooks.DecisionDeny || result.Halt || result.RequiredFailure {
		RejectTool(ctx)
		return ctx, call, result, false
	}
	if result.UpdatedInput != "" {
		call.Input = result.UpdatedInput
	}
	if state, _ := ctx.Value(actionKey{}).(*actionState); state != nil {
		state.mu.Lock()
		state.input = call.Input
		state.mu.Unlock()
	}
	if result.Decision == hooks.DecisionAllow {
		ctx = permission.WithHookApproval(ctx, call.ID)
	}
	return ctx, call, result, true
}

// ObserveTool surrounds the actual guarded tool boundary. It records one
// terminal event even when a plan guard, permission, error, or cancel prevents
// the inner tool from completing.
func (r *Runtime) ObserveTool(ctx context.Context, sid string, call fantasy.ToolCall, run func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error)) (response fantasy.ToolResponse, retErr error) {
	state := &actionState{input: call.Input}
	ctx = context.WithValue(ctx, actionKey{}, state)
	started := time.Now()
	defer func() {
		panicked := recover()
		state.mu.Lock()
		entered, rejected, actualInput := state.entered, state.rejected, state.input
		state.mu.Unlock()
		info := details(ctx)
		info.ToolCallID, info.DurationMS = call.ID, time.Since(started).Milliseconds()
		info.Outcome = "completed"
		event := hooks.EventPostToolUse
		var metadata struct {
			ExitCode *int `json:"exit_code"`
		}
		_ = json.Unmarshal([]byte(response.Metadata), &metadata)
		info.ExitCode = metadata.ExitCode
		switch {
		case panicked != nil:
			event, info.Outcome, info.Error = hooks.EventPostToolUseFailure, "failed", "tool panicked"
		case ctx.Err() != nil || errors.Is(retErr, context.Canceled) || errors.Is(retErr, context.DeadlineExceeded):
			event, info.Outcome, info.IsInterrupt = hooks.EventToolCancelled, "cancelled", true
		case rejected || !entered:
			event, info.Outcome = hooks.EventActionRejected, "rejected"
		case retErr != nil || response.IsError || metadata.ExitCode != nil && *metadata.ExitCode != 0:
			event, info.Outcome = hooks.EventPostToolUseFailure, "failed"
		}
		if retErr != nil {
			info.Error = retErr.Error()
		}
		info.ToolResponse, _ = json.Marshal(map[string]any{"content": response.Content, "is_error": response.IsError, "metadata": response.Metadata})
		input, err := sjson.Set(actualInput, "response", response.Content)
		if err != nil {
			input = actualInput
		}
		result, dispatchErr := r.Run(ctx, event, sid, call.Name, input, info)
		if dispatchErr != nil {
			slog.Warn("Terminal hook dispatch failed", "event", event, "tool", call.Name)
		}
		if result.Context != "" {
			if response.Content != "" {
				response.Content += "\n"
			}
			response.Content += result.Context
		}
		if result.RequiredFailure {
			response.IsError = true
			response.Content += "\nRequired post-action hook failed; the observed tool effects have already occurred."
		}
		response.Metadata = MergeMetadata(response.Metadata, result)
		if panicked != nil {
			panic(panicked)
		}
	}()
	return run(ctx, call)
}

// ExecuteTool is the shared path for explicit native shell actions.
func (r *Runtime) ExecuteTool(ctx context.Context, sid string, call fantasy.ToolCall, run func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error)) (fantasy.ToolResponse, error) {
	return r.ObserveTool(ctx, sid, call, func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ctx, call, result, allowed := r.Before(ctx, sid, call)
		if !allowed {
			response := fantasy.NewTextErrorResponse("Tool call blocked by hook. Reason: " + result.Reason)
			response.StopTurn = result.Halt
			response.Metadata = MergeMetadata("", result)
			return response, nil
		}
		EnterTool(ctx)
		response, err := run(ctx, call)
		if result.Context != "" {
			response.Content += "\n" + result.Context
		}
		response.Metadata = MergeMetadata(response.Metadata, result)
		return response, err
	})
}

func MergeMetadata(existing string, result hooks.AggregateResult) string {
	if result.HookCount == 0 {
		return existing
	}
	if existing == "" {
		existing = "{}"
	}
	value := hooks.HookMetadata{HookCount: result.HookCount, Decision: result.Decision.String(), Halt: result.Halt, Reason: result.Reason, InputRewrite: result.UpdatedInput != "", Hooks: result.Hooks}
	var old struct {
		Hook hooks.HookMetadata `json:"hook"`
	}
	if json.Unmarshal([]byte(existing), &old) == nil && old.Hook.HookCount != 0 {
		value.HookCount += old.Hook.HookCount
		value.Hooks = append(old.Hook.Hooks, value.Hooks...)
		value.Halt = value.Halt || old.Hook.Halt
		value.InputRewrite = value.InputRewrite || old.Hook.InputRewrite
		if old.Hook.Decision != "" && old.Hook.Decision != "none" {
			value.Decision = old.Hook.Decision
		}
		if old.Hook.Reason != "" {
			value.Reason = old.Hook.Reason
		}
	}
	merged, err := sjson.Set(existing, "hook", value)
	if err != nil {
		return existing
	}
	return merged
}

// ObservePermission receives native decisions and cannot answer a request.
func (r *Runtime) ObservePermission(ctx context.Context, observation permission.Observation) {
	if !observation.Requested && !observation.Granted {
		RejectTool(ctx)
	}
	event, outcome := hooks.EventPermissionResolved, "denied"
	if observation.Requested {
		event, outcome = hooks.EventPermissionRequest, "requested"
	} else if observation.Granted {
		outcome = "granted"
	}
	if observation.Cancelled {
		outcome = "cancelled"
	}
	info := details(ctx)
	info.ToolCallID, info.Outcome = observation.Request.ToolCallID, outcome
	input, _ := json.Marshal(map[string]any{"permission_id": observation.Request.ID, "action": observation.Request.Action, "path": observation.Request.Path, "granted": observation.Granted, "cancelled": observation.Cancelled})
	_, err := r.Run(ctx, event, observation.Request.SessionID, observation.Request.ToolName, string(input), info)
	if err != nil {
		slog.Warn("Permission observation hook dispatch failed", "event", event)
	}
}

// Close cancels and drains application-owned observers before DB release.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closed = true
	runners := append([]*hooks.Runner(nil), r.runners...)
	r.mu.Unlock()
	r.cancel()
	for _, runner := range runners {
		runner.CancelAllAsync()
	}
	var result error
	done := make(chan struct{})
	go func() { r.inflight.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		result = ctx.Err()
	}
	for _, runner := range runners {
		result = errors.Join(result, runner.DrainAsync(ctx))
	}
	return result
}
