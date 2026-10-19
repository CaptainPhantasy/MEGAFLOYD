package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/legacy-ai/floyd/internal/message"

	"charm.land/fantasy"
)

// SwarmCoordinator extends the single-agent Coordinator to support multi-agent
// task dispatch. It wraps the existing Coordinator and adds a task queue,
// agent assignment, and result collection protocol.
//
// Usage:
//
//	swarm := NewSwarmCoordinator(coordinator)
//	swarm.RegisterAgent("researcher", researcherCoordinator)
//	taskID := swarm.Submit("investigate X", "researcher")
//	result := swarm.Await(taskID)
type SwarmCoordinator struct {
	inner  Coordinator
	agents map[string]Coordinator
	mu     sync.RWMutex

	pending map[string]*SwarmTask
	nextID  int
	taskMu  sync.Mutex
}

// SwarmTask represents a dispatched task in the swarm.
type SwarmTask struct {
	ID        string
	Prompt    string
	AgentName string
	Result    *SwarmResult
	Error     error
	done      chan struct{}
}

// SwarmResult holds the outcome of a swarm task.
type SwarmResult struct {
	AgentName string
	Output    string
	SessionID string
}

// NewSwarmCoordinator creates a new swarm coordinator wrapping the given
// primary coordinator. The primary coordinator serves as the default agent
// for tasks that don't specify a particular agent.
func NewSwarmCoordinator(inner Coordinator) *SwarmCoordinator {
	return &SwarmCoordinator{
		inner:   inner,
		agents:  make(map[string]Coordinator),
		pending: make(map[string]*SwarmTask),
		nextID:  1,
	}
}

// RegisterAgent adds a named agent coordinator to the swarm.
// The agent name can then be used in Submit to route tasks.
func (sc *SwarmCoordinator) RegisterAgent(name string, agent Coordinator) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.agents[name] = agent
	slog.Info("Swarm agent registered", "agent", name)
}

// Agents returns the list of registered agent names.
func (sc *SwarmCoordinator) Agents() []string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	names := make([]string, 0, len(sc.agents))
	for name := range sc.agents {
		names = append(names, name)
	}
	return names
}

// Submit enqueues a task for execution by the named agent.
// If agentName is empty, the primary (inner) coordinator is used.
// Returns the task ID which can be used with Await.
func (sc *SwarmCoordinator) Submit(prompt, agentName string, attachments ...message.Attachment) string {
	sc.taskMu.Lock()
	id := fmt.Sprintf("swarm-%d", sc.nextID)
	sc.nextID++
	sc.taskMu.Unlock()

	task := &SwarmTask{
		ID:        id,
		Prompt:    prompt,
		AgentName: agentName,
		done:      make(chan struct{}),
	}

	sc.taskMu.Lock()
	sc.pending[id] = task
	sc.taskMu.Unlock()

	// Launch execution asynchronously
	go sc.executeTask(task, attachments)

	slog.Debug("Swarm task submitted", "id", id, "agent", agentName)
	return id
}

// Await blocks until the task with the given ID completes, then returns the result.
func (sc *SwarmCoordinator) Await(taskID string) (*SwarmResult, error) {
	sc.taskMu.Lock()
	task, ok := sc.pending[taskID]
	sc.taskMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	<-task.done

	sc.taskMu.Lock()
	delete(sc.pending, taskID)
	sc.taskMu.Unlock()

	return task.Result, task.Error
}

// executeTask runs a task on the appropriate agent.
func (sc *SwarmCoordinator) executeTask(task *SwarmTask, attachments []message.Attachment) {
	defer close(task.done)

	agent := sc.resolveAgent(task.AgentName)
	if agent == nil {
		task.Error = fmt.Errorf("no agent registered with name %q", task.AgentName)
		return
	}

	ctx := context.Background()
	sessionID := generateSessionID()

	result, err := agent.Run(ctx, sessionID, task.Prompt, attachments...)
	if err != nil {
		task.Error = fmt.Errorf("agent %q task failed: %w", task.AgentName, err)
		return
	}

	task.Result = &SwarmResult{
		AgentName: task.AgentName,
		Output:    extractText(result),
		SessionID: sessionID,
	}
}

// resolveAgent returns the coordinator for the named agent, falling back to inner.
func (sc *SwarmCoordinator) resolveAgent(name string) Coordinator {
	if name == "" || name == "primary" || name == "default" {
		return sc.inner
	}
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if agent, ok := sc.agents[name]; ok {
		return agent
	}
	// Fall back to primary if named agent not found
	slog.Warn("Named agent not found, falling back to primary", "agent", name)
	return sc.inner
}

func generateSessionID() string {
	return fmt.Sprintf("swarm-sess-%d", nextSessionCounter())
}

// sessionCounter provides goroutine-safe monotonically increasing session IDs
// for swarm task sessions. Concurrent Submit() calls each launch a goroutine,
// so the counter must be accessed atomically.
var sessionCounter atomic.Int64

func nextSessionCounter() int64 {
	return sessionCounter.Add(1)
}

// extractText extracts the text output from a fantasy.AgentResult.
func extractText(result *fantasy.AgentResult) string {
	if result == nil {
		return ""
	}
	return result.Response.Content.Text()
}
