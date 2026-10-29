package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/filelock"
)

// EventDetails is supplied only by native dispatch, never decoded from tool input.
type EventDetails struct {
	EventID          string          `json:"event_id"`
	Timestamp        string          `json:"timestamp"`
	ToolCallID       string          `json:"tool_use_id,omitempty"`
	RunID            string          `json:"run_id,omitempty"`
	ParentToolCallID string          `json:"parent_tool_use_id,omitempty"`
	Outcome          string          `json:"outcome,omitempty"`
	DurationMS       int64           `json:"duration_ms,omitempty"`
	ExitCode         *int            `json:"exit_code,omitempty"`
	ToolResponse     json.RawMessage `json:"tool_response,omitempty"`
	Error            string          `json:"error,omitempty"`
	IsInterrupt      bool            `json:"is_interrupt,omitempty"`
}
type (
	eventDetailsKey struct{}
	asyncContextKey struct{}
)

// WithAsyncContext supplies the owner lifetime for async observers. Synchronous
// checks still use ctx cancellation. Values/identity remain from the call context.
func WithAsyncContext(ctx, lifetime context.Context) context.Context {
	return context.WithValue(ctx, asyncContextKey{}, lifetime)
}

func WithEventDetails(ctx context.Context, details EventDetails) context.Context {
	details.ToolResponse = append(json.RawMessage(nil), details.ToolResponse...)
	if details.ExitCode != nil {
		code := *details.ExitCode
		details.ExitCode = &code
	}
	return context.WithValue(ctx, eventDetailsKey{}, details)
}

func ensureEventDetails(ctx context.Context) context.Context {
	d, _ := ctx.Value(eventDetailsKey{}).(EventDetails)
	if d.EventID == "" {
		d.EventID = uuid.NewString()
	}
	if d.Timestamp == "" {
		d.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return WithEventDetails(ctx, d)
}

// Execution contains audit metadata only. Reasons, command/input/output bodies
// are intentionally absent. A running and terminal row share ID.
type Execution struct {
	Outcome          string `json:"outcome,omitempty"`
	ID               string `json:"id"`
	EventID          string `json:"event_id"`
	Event            string `json:"event"`
	SessionID        string `json:"session_id"`
	ToolName         string `json:"tool_name,omitempty"`
	ToolCallID       string `json:"tool_call_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`
	ConfigIndex      int    `json:"config_index"`
	HookName         string `json:"hook_name"`
	CommandHash      string `json:"command_hash"`
	Mode             string `json:"mode"`
	Required         bool   `json:"required"`
	State            string `json:"state"`
	StartedAt        int64  `json:"started_at"`
	FinishedAt       int64  `json:"finished_at,omitempty"`
	Decision         string `json:"decision"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Rewritten        bool   `json:"rewritten"`
	ErrorKind        string `json:"error_kind,omitempty"`
	AsyncJobID       string `json:"async_job_id,omitempty"`
}

func (r *Runner) SetRecorder(record func(Execution)) { r.mu.Lock(); r.recorder = record; r.mu.Unlock() }

func (r *Runner) record(e Execution) {
	r.mu.Lock()
	f := r.recorder
	r.mu.Unlock()
	if f != nil {
		if e.ExitCode != nil {
			v := *e.ExitCode
			e.ExitCode = &v
		}
		f(e)
	}
}

func resultState(result HookResult) string {
	switch {
	case result.Cancelled:
		return "cancelled"
	case result.TimedOut:
		return "timed_out"
	case result.ExecutionError != "":
		return "failed"
	case result.Decision == DecisionDeny || result.Halt:
		return "denied"
	default:
		return "succeeded"
	}
}

type AsyncJob struct {
	ID         string     `json:"id"`
	EventID    string     `json:"event_id"`
	Event      string     `json:"event"`
	SessionID  string     `json:"session_id"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Hook       string     `json:"hook"`
	State      string     `json:"state"`
	StartedAt  int64      `json:"started_at"`
	FinishedAt int64      `json:"finished_at,omitempty"`
	Result     HookResult `json:"result"`
}
type asyncState struct {
	job    AsyncJob
	cancel context.CancelFunc
	done   chan struct{}
}

func cloneJob(j AsyncJob) AsyncJob {
	if j.Result.ExitCode != nil {
		v := *j.Result.ExitCode
		j.Result.ExitCode = &v
	}
	return j
}

func (r *Runner) startAsync(ctx context.Context, event, sid, tool, input string, h compiledHook, mode string) string {
	ctx = filelock.WithoutObserverLease(ctx)
	lifetime, hasLifetime := ctx.Value(asyncContextKey{}).(context.Context)
	if hasLifetime {
		ctx = context.WithoutCancel(ctx)
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := func() bool { return true }
	if hasLifetime {
		stop = context.AfterFunc(lifetime, cancel)
	}
	d, _ := ctx.Value(eventDetailsKey{}).(EventDetails)
	id := uuid.NewString()
	state := &asyncState{job: AsyncJob{ID: id, EventID: d.EventID, Event: event, SessionID: sid, ToolCallID: d.ToolCallID, Hook: safeName(h), State: "running", StartedAt: time.Now().UnixNano()}, cancel: cancel, done: make(chan struct{})}
	r.mu.Lock()
	if r.async == nil {
		r.async = map[string]*asyncState{}
	}
	r.async[id] = state
	r.mu.Unlock()
	go func() {
		defer stop()
		defer cancel()
		result := r.execute(ctx, event, sid, tool, input, h, mode, id)
		r.mu.Lock()
		state.job.Result = result
		state.job.State = resultState(result)
		state.job.FinishedAt = time.Now().UnixNano()
		close(state.done)
		r.mu.Unlock()
	}()
	return id
}

func (r *Runner) AsyncResults() []AsyncJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AsyncJob, 0, len(r.async))
	for _, s := range r.async {
		out = append(out, cloneJob(s.job))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt < out[j].StartedAt
	})
	return out
}

func (r *Runner) CancelAsync(id string) bool {
	r.mu.Lock()
	s, ok := r.async[id]
	if ok {
		ok = s.job.State == "running"
	}
	r.mu.Unlock()
	if ok {
		s.cancel()
	}
	return ok
}

func (r *Runner) AwaitAsync(ctx context.Context, id string) (AsyncJob, error) {
	r.mu.Lock()
	s, ok := r.async[id]
	r.mu.Unlock()
	if !ok {
		return AsyncJob{}, errors.New("unknown hook job")
	}
	select {
	case <-ctx.Done():
		return AsyncJob{}, ctx.Err()
	case <-s.done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return cloneJob(s.job), nil
	}
}

func (r *Runner) CancelAllAsync() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.async {
		if s.job.State == "running" {
			s.cancel()
		}
	}
}

// DrainAsync waits for the jobs present at this lifecycle boundary. The owner
// must stop new dispatch first; cancelling the wait does not kill jobs.
func (r *Runner) DrainAsync(ctx context.Context) error {
	r.mu.Lock()
	waits := make([]<-chan struct{}, 0, len(r.async))
	for _, s := range r.async {
		waits = append(waits, s.done)
	}
	r.mu.Unlock()
	for _, ch := range waits {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
	return nil
}

// ForgetAsync releases a completed in-memory observer after its durable record
// has been consumed by the runtime. Running jobs cannot be forgotten.
func (r *Runner) ForgetAsync(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.async[id]
	if !ok || state.job.State == "running" {
		return false
	}
	delete(r.async, id)
	return true
}
