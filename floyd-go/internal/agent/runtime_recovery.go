package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/plan"
	"github.com/legacy-ai/floyd/internal/session"
)

const runtimeAgentMetadataKey = "runtime_agent"

type runtimeAgentSnapshot struct {
	Version    int             `json:"version"`
	Definition config.Agent    `json:"definition"`
	Profile    *RuntimeProfile `json:"profile"`
	// Explicit fields preserve nil versus empty allowlists across serialization.
	Tools []string            `json:"tools"`
	MCP   map[string][]string `json:"mcp"`
}

func newRuntimeAgentSnapshot(definition config.Agent, profile *RuntimeProfile) runtimeAgentSnapshot {
	result := runtimeAgentSnapshot{Version: 1, Definition: definition, Tools: slices.Clone(definition.AllowedTools), MCP: maps.Clone(definition.AllowedMCP)}
	if profile != nil {
		copy := *profile
		copy.AllowedTools = slices.Clone(profile.AllowedTools)
		result.Profile = &copy
	}
	return result
}

// restoreRuntimeChild reconstructs the saved restricted engine before accepting
// a prompt. Missing or malformed identity never falls through to the coder.
func (c *coordinator) restoreRuntimeChild(ctx context.Context, id string) (SessionAgent, bool, error) {
	if child, ok := c.RuntimeChildAgent(id); ok {
		return child, true, nil
	}
	if c.sessions == nil {
		return nil, false, nil
	}
	stored, err := c.sessions.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	_, savedIdentity := stored.Metadata[runtimeAgentMetadataKey]
	if !c.sessions.IsAgentToolSession(id) && !savedIdentity {
		return nil, false, nil
	}
	c.runtimeControlMu.Lock()
	defer c.runtimeControlMu.Unlock()
	if child, ok := c.RuntimeChildAgent(id); ok {
		return child, true, nil
	}
	raw, ok := stored.Metadata[runtimeAgentMetadataKey]
	if !ok {
		return nil, true, errors.New("saved child agent identity is unavailable; refusing to resume it with coder permissions")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, true, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		return nil, true, fmt.Errorf("read saved child agent identity: %w", err)
	}
	if _, ok := fields["tools"]; !ok {
		return nil, true, errors.New("saved child tool restrictions are missing")
	}
	if _, ok := fields["mcp"]; !ok {
		return nil, true, errors.New("saved child MCP restrictions are missing")
	}
	var snapshot runtimeAgentSnapshot
	if err = json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, true, fmt.Errorf("read saved child agent identity: %w", err)
	}
	if snapshot.Version != 1 || snapshot.Definition.ID == "" || (snapshot.Definition.Model != config.SelectedModelTypeLarge && snapshot.Definition.Model != config.SelectedModelTypeSmall) {
		return nil, true, errors.New("saved child agent identity is invalid")
	}
	snapshot.Definition.AllowedTools = snapshot.Tools
	snapshot.Definition.AllowedMCP = snapshot.MCP
	if configured, ok := c.cfg.Config().Agents[snapshot.Definition.ID]; ok && configured.Disabled {
		return nil, true, errors.New("saved child agent is disabled")
	}
	child, err := c.buildRuntimeAgent(ctx, snapshot.Definition, snapshot.Profile)
	if err != nil {
		return nil, true, err
	}
	name := snapshot.Definition.ID
	if snapshot.Profile != nil {
		name = snapshot.Profile.ID
	}
	task := &runtimeTask{agent: child, cancel: func() {}, detach: make(chan struct{}), done: make(chan struct{}), detached: true, view: RuntimeTask{ID: id, SessionID: stored.ParentSessionID, Kind: "subagent", Description: stored.Title, Status: "restored", CreatedAt: time.Unix(stored.CreatedAt, 0), AgentID: id, SubagentType: name, Model: child.Model().ModelCfg.Model, RunInBackground: true}}
	close(task.detach)
	close(task.done)
	store := c.runtimeTasks()
	store.Lock()
	store.tasks[id] = task
	store.Unlock()
	return child, true, nil
}

func (c *coordinator) recordRuntimeFollowup(id string, result *fantasy.AgentResult, err error) {
	if (result == nil && err == nil) || errors.Is(err, ErrSessionBusy) {
		return
	}
	store := c.runtimeTasks()
	store.Lock()
	task := store.tasks[id]
	store.Unlock()
	if task == nil {
		return
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	finished := time.Now().UTC()
	task.view.CompletedAt = &finished
	task.view.Status = "completed"
	task.view.OutputPreview = subAgentOutput(result)
	if err != nil {
		task.view.Status = "failed"
		task.view.OutputPreview = err.Error()
	}
	if errors.Is(err, context.Canceled) {
		task.view.Status = "cancelled"
	}
	task.view.OutputBytes = len(task.view.OutputPreview)
}

// Persist the child identity and inherit the parent's plan restriction before
// the first tool can run. Failed preparation leaves a recoverable empty session.
func (c *coordinator) prepareRuntimeChild(ctx context.Context, childID string, params subAgentParams) error {
	if params.RuntimeDefinition != nil {
		mutator, ok := c.sessions.(session.MetadataMutator)
		if !ok {
			return errors.New("child agent identity cannot be persisted")
		}
		if _, err := mutator.MutateMetadata(ctx, childID, func(metadata map[string]any) error {
			metadata[runtimeAgentMetadataKey] = params.RuntimeDefinition
			return nil
		}); err != nil {
			return fmt.Errorf("persist child agent identity: %w", err)
		}
	}
	if c.cfg == nil || c.cfg.Config() == nil || c.cfg.Config().Options == nil {
		return errors.New("child plan restrictions require workspace configuration")
	}
	parentPlan, err := c.planService().Status(params.SessionID)
	if err != nil {
		return fmt.Errorf("read parent plan restriction: %w", err)
	}
	if parentPlan.Active {
		if _, err := c.planService().Enter(childID); err != nil && !errors.Is(err, plan.ErrAlreadyActive) {
			return fmt.Errorf("inherit parent plan restriction: %w", err)
		}
	}
	return nil
}
