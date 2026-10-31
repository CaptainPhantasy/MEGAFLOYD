package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

type RuntimeTask struct {
	ID               string     `json:"id"`
	SessionID        string     `json:"session_id"`
	Kind             string     `json:"kind"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Command          string     `json:"command,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	OutputPreview    string     `json:"output_preview,omitempty"`
	OutputBytes      int        `json:"output_bytes"`
	Model            string     `json:"model,omitempty"`
	AgentID          string     `json:"agent_id,omitempty"`
	SubagentType     string     `json:"subagent_type,omitempty"`
	ParentToolCallID string     `json:"parent_tool_call_id,omitempty"`
	RunInBackground  bool       `json:"run_in_background"`
}
type runtimeSubagentResult struct {
	response fantasy.ToolResponse
	err      error
}
type runtimeTask struct {
	mu           sync.Mutex
	agent        SessionAgent
	view         RuntimeTask
	cancel       context.CancelFunc
	detach, done chan struct{}
	detached     bool
}
type runtimeTasksState struct {
	sync.Mutex
	tasks   map[string]*runtimeTask
	active  map[string]int
	changed chan struct{}
}

var runtimeTaskStores sync.Map // *coordinator -> *runtimeTasksState
func (c *coordinator) runtimeTasks() *runtimeTasksState {
	value, _ := runtimeTaskStores.LoadOrStore(c, &runtimeTasksState{tasks: map[string]*runtimeTask{}, active: map[string]int{}, changed: make(chan struct{})})
	return value.(*runtimeTasksState)
}

type SwarmState struct {
	Active      bool `json:"active"`
	MaxParallel int  `json:"max_parallel"`
}

func RuntimeSwarm(ctx context.Context, svc session.Service, id string) (SwarmState, error) {
	s, err := svc.Get(ctx, id)
	if err != nil {
		return SwarmState{}, err
	}
	out := SwarmState{MaxParallel: 3}
	if v := s.Metadata["swarm"]; v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return out, err
		}
		if err = json.Unmarshal(b, &out); err != nil {
			return out, err
		}
	}
	if out.MaxParallel < 1 || out.MaxParallel > 8 {
		return out, errors.New("stored swarm parallelism is invalid")
	}
	return out, nil
}

func SetRuntimeSwarm(ctx context.Context, svc session.Service, id string, active *bool, parallel *int) (SwarmState, error) {
	m, ok := svc.(session.MetadataMutator)
	if !ok {
		return SwarmState{}, errors.New("session metadata is unavailable")
	}
	out := SwarmState{MaxParallel: 3}
	_, err := m.MutateMetadata(ctx, id, func(meta map[string]any) error {
		if v := meta["swarm"]; v != nil {
			b, e := json.Marshal(v)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(b, &out); e != nil {
				return e
			}
		}
		if active != nil {
			out.Active = *active
		}
		if parallel != nil {
			out.MaxParallel = *parallel
		}
		if out.MaxParallel < 1 || out.MaxParallel > 8 {
			return errors.New("max_parallel must be between 1 and 8")
		}
		meta["swarm"] = map[string]any{"active": out.Active, "max_parallel": out.MaxParallel}
		return nil
	})
	return out, err
}

func (c *coordinator) acquireRuntimeTask(ctx context.Context, sessionID string) (func(), error) {
	store := c.runtimeTasks()
	for {
		// Pair the metadata snapshot with the wake channel under one lock so
		// a concurrent /swarm change cannot be lost before we begin waiting.
		store.Lock()
		swarm, err := RuntimeSwarm(ctx, c.sessions, sessionID)
		if err != nil {
			store.Unlock()
			return nil, err
		}
		limit := 1
		if swarm.Active {
			limit = swarm.MaxParallel
		}
		if store.active[sessionID] < limit {
			store.active[sessionID]++
			store.Unlock()
			return func() {
				store.Lock()
				store.active[sessionID]--
				close(store.changed)
				store.changed = make(chan struct{})
				store.Unlock()
			}, nil
		}
		changed := store.changed
		store.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// runSubAgent tracks the existing child-session execution. Detach releases the
// parent tool call while this same child continues; cancel targets its context.
func (c *coordinator) startRuntimeSubagent(ctx context.Context, params subAgentParams, background bool) (*runtimeTask, <-chan runtimeSubagentResult) {
	id := c.sessions.CreateAgentToolSessionID(params.AgentMessageID, params.ToolCallID)
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	now := time.Now().UTC()
	task := &runtimeTask{agent: params.Agent, view: RuntimeTask{ID: id, SessionID: params.SessionID, Kind: "subagent", Description: params.SessionTitle, Status: "queued", CreatedAt: now, AgentID: id, ParentToolCallID: params.ToolCallID, Model: params.Agent.Model().ModelCfg.Model}, cancel: cancel, detach: make(chan struct{}), done: make(chan struct{})}
	if background {
		task.detached = true
		task.view.RunInBackground = true
		close(task.detach)
	}
	store := c.runtimeTasks()
	store.Lock()
	store.tasks[id] = task
	store.Unlock()
	go func() {
		select {
		case <-ctx.Done():
			task.mu.Lock()
			if !task.detached {
				cancel()
			}
			task.mu.Unlock()
		case <-task.done:
		}
	}()
	resultCh := make(chan runtimeSubagentResult, 1)
	go func() {
		defer cancel()
		defer close(task.done)
		release, err := c.acquireRuntimeTask(runCtx, params.SessionID)
		var response fantasy.ToolResponse
		if err == nil {
			defer release()
			started := time.Now().UTC()
			task.mu.Lock()
			task.view.StartedAt = &started
			task.view.Status = "running"
			task.mu.Unlock()
			response, err = c.runSubAgentInline(runCtx, params)
		}
		finished := time.Now().UTC()
		task.mu.Lock()
		task.view.CompletedAt = &finished
		task.view.Status = "completed"
		if err != nil || response.IsError {
			task.view.Status = "failed"
		}
		if runCtx.Err() != nil {
			task.view.Status = "cancelled"
		}
		task.view.OutputPreview = response.Content
		task.view.OutputBytes = len(response.Content)
		if err != nil {
			task.view.OutputPreview = err.Error()
			task.view.OutputBytes = len(err.Error())
		}
		task.mu.Unlock()
		resultCh <- runtimeSubagentResult{response, err}
	}()
	return task, resultCh
}

func (c *coordinator) runSubAgent(ctx context.Context, params subAgentParams) (fantasy.ToolResponse, error) {
	task, resultCh := c.startRuntimeSubagent(ctx, params, false)
	id := task.view.ID
	select {
	case r := <-resultCh:
		return r.response, r.err
	case <-task.detach:
		return fantasy.NewTextResponse(fmt.Sprintf("Subagent continues in background. Task ID: %s. Child session: %s. Use the tasks control to inspect or cancel it.", id, id)), nil
	case <-ctx.Done():
		return fantasy.ToolResponse{}, ctx.Err()
	}
}

func (c *coordinator) RuntimeTasks() []RuntimeTask {
	store := c.runtimeTasks()
	store.Lock()
	tasks := make([]*runtimeTask, 0, len(store.tasks))
	for _, task := range store.tasks {
		tasks = append(tasks, task)
	}
	store.Unlock()
	out := make([]RuntimeTask, 0, len(tasks))
	for _, task := range tasks {
		task.mu.Lock()
		item := task.view
		if task.agent.IsSessionBusy(item.AgentID) && item.Status != "running" {
			item.Status = "running"
			item.CompletedAt = nil
		}
		task.mu.Unlock()
		// Only expose the same reviewed projection used by the conversation UI.
		// A running task's persisted assistant output is useful progress even
		// before its final response exists.
		if item.Status == "running" && c.messages != nil {
			if messages, err := message.PublicList(context.Background(), c.messages, item.AgentID); err == nil {
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == message.Assistant && messages[i].Content().Text != "" {
						item.OutputPreview = messages[i].Content().Text
						item.OutputBytes = len(item.OutputPreview)
						break
					}
				}
			}
		}
		if len(item.OutputPreview) > 16384 {
			item.OutputPreview = item.OutputPreview[len(item.OutputPreview)-16384:]
		}
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b RuntimeTask) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out
}

func (c *coordinator) RuntimeTaskControl(id, action string) error {
	store := c.runtimeTasks()
	store.Lock()
	task := store.tasks[id]
	store.Unlock()
	if task == nil {
		return fmt.Errorf("task %q not found", id)
	}
	if action != "cancel" && action != "detach" {
		return errors.New("unknown task control")
	}
	task.mu.Lock()
	if action == "cancel" {
		task.cancel()
		task.mu.Unlock()
		task.agent.Cancel(id)
		return nil
	}
	if (task.view.Status == "running" || task.view.Status == "queued") && !task.detached {
		task.detached = true
		task.view.RunInBackground = true
		close(task.detach)
	}
	task.mu.Unlock()
	return nil
}

func (c *coordinator) RuntimeCancelTasks() {
	store := c.runtimeTasks()
	store.Lock()
	tasks := make([]*runtimeTask, 0, len(store.tasks))
	for _, task := range store.tasks {
		tasks = append(tasks, task)
	}
	store.Unlock()
	for _, task := range tasks {
		task.cancel()
		task.agent.CancelAll()
	}
}

// RuntimeChildAgent selects the same child engine for follow-up messages, cancellation and queue controls.
func (c *coordinator) RuntimeChildAgent(id string) (SessionAgent, bool) {
	store := c.runtimeTasks()
	store.Lock()
	task := store.tasks[id]
	store.Unlock()
	if task == nil {
		return nil, false
	}
	return task.agent, true
}

func (c *coordinator) runtimeSessionAgent(id string) SessionAgent {
	if child, ok := c.RuntimeChildAgent(id); ok {
		return child
	}
	return c.currentAgent
}

func (c *coordinator) runtimeChildrenBusy() bool {
	store := c.runtimeTasks()
	store.Lock()
	tasks := make([]*runtimeTask, 0, len(store.tasks))
	for _, t := range store.tasks {
		tasks = append(tasks, t)
	}
	store.Unlock()
	for _, t := range tasks {
		t.mu.Lock()
		running := t.view.Status == "running" || t.view.Status == "queued"
		t.mu.Unlock()
		if running || t.agent.IsBusy() {
			return true
		}
	}
	return false
}

func (c *coordinator) runtimeSessionBusy(id string) bool {
	store := c.runtimeTasks()
	store.Lock()
	task := store.tasks[id]
	store.Unlock()
	if task != nil {
		task.mu.Lock()
		running := task.view.Status == "running" || task.view.Status == "queued"
		task.mu.Unlock()
		return running || task.agent.IsSessionBusy(id)
	}
	return c.currentAgent.IsSessionBusy(id)
}

// RuntimeSwarmChanged wakes queued children when a user changes parallelism.
func (c *coordinator) RuntimeSwarmChanged() {
	store := c.runtimeTasks()
	store.Lock()
	close(store.changed)
	store.changed = make(chan struct{})
	store.Unlock()
}
