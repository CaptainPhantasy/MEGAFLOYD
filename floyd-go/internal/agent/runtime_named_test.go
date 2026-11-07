package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

type namedModelRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages json.RawMessage `json:"messages"`
	Tools    []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

// Drive the real provider, child engine and SQLite sessions. Named dispatch
// must select both the model and enforced tool surface, not merely label a run.
func TestNamedSwarmRunsDistinctModelsToolsAndSessions(t *testing.T) {
	c := newToolSetTestCoordinator(t)
	requests := make(chan namedModelRequest, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request namedModelRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"title","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"Fixture title"},"finish_reason":"stop"}]}`, request.Model)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Checked progress. \"}}]}\n\n", request.Model)
		w.(http.Flusher).Flush()
		if len(request.Tools) > 0 {
			requests <- request
		}
		if len(request.Tools) > 0 {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", request.Model, "Completed "+request.Model+".")
	}))
	t.Cleanup(provider.Close)
	t.Cleanup(unblock)
	t.Cleanup(func() { c.RuntimeCancelTasks(); runtimeTaskStores.Delete(c) })
	cfg := c.cfg.Config()
	p, _ := cfg.Providers.Get("mock")
	p.Type = "openai-compat"
	p.BaseURL = provider.URL + "/v1"
	p.Models = []catwalk.Model{{ID: "writer-model", Name: "Writer", ContextWindow: 32768, DefaultMaxTokens: 128}, {ID: "reader-model", Name: "Reader", ContextWindow: 32768, DefaultMaxTokens: 128}}
	cfg.Providers.Set("mock", p)
	cfg.Models[config.SelectedModelTypeLarge] = config.SelectedModel{Provider: "mock", Model: "writer-model"}
	cfg.Models[config.SelectedModelTypeSmall] = config.SelectedModel{Provider: "mock", Model: "reader-model"}
	reader := cfg.Agents[config.AgentTask]
	reader.Model = config.SelectedModelTypeSmall
	cfg.Agents[config.AgentTask] = reader
	parent, err := c.sessions.Create(t.Context(), "named swarm")
	require.NoError(t, err)
	on, parallel := true, 2
	_, err = SetRuntimeSwarm(t.Context(), c.sessions, parent.ID, &on, &parallel)
	require.NoError(t, err)
	first, err := c.RuntimeStartSubagent(t.Context(), parent.ID, "Inspect writer work", config.AgentCoder, "")
	require.NoError(t, err)
	second, err := c.RuntimeStartSubagent(t.Context(), parent.ID, "Inspect reader work", config.AgentTask, "")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "writer-model", first.Model)
	require.Equal(t, "reader-model", second.Model)
	observed := map[string][]string{}
	for range 2 {
		select {
		case request := <-requests:
			for _, tool := range request.Tools {
				observed[request.Model] = append(observed[request.Model], tool.Function.Name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("both named agents did not run concurrently: %+v", c.RuntimeTasks())
		}
	}
	require.Contains(t, observed["writer-model"], "write")
	require.Contains(t, observed["reader-model"], "view")
	for _, name := range []string{"write", "edit", "bash", "parallel_bash", "apply_patch"} {
		require.NotContains(t, observed["reader-model"], name)
	}
	require.Eventually(t, func() bool {
		for _, task := range c.RuntimeTasks() {
			if task.Status == "running" && strings.Contains(task.OutputPreview, "Checked progress.") {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
	unblock()
	require.Eventually(t, func() bool {
		tasks := c.RuntimeTasks()
		return len(tasks) == 2 && tasks[0].Status == "completed" && tasks[1].Status == "completed"
	}, 5*time.Second, 10*time.Millisecond)
	for _, task := range c.RuntimeTasks() {
		require.Contains(t, task.OutputPreview, "Completed "+task.Model+".")
		child, err := c.sessions.Get(t.Context(), task.ID)
		require.NoError(t, err)
		require.Equal(t, parent.ID, child.ParentSessionID)
		messages, err := c.messages.List(t.Context(), task.ID)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(messages), 2)
	}
	names := []string{first.SubagentType, second.SubagentType}
	slices.Sort(names)
	require.Equal(t, []string{"coder", "task"}, names)
	_, err = c.RuntimeSaveProfile("fixture-reader", RuntimeProfile{Description: "Read only fixture", SystemPrompt: "ROLE_FIXTURE_READ_ONLY", Model: config.SelectedModelTypeSmall, AllowedTools: []string{"view"}})
	require.NoError(t, err)
	profiled, err := c.RuntimeStartSubagent(t.Context(), parent.ID, "Read with the named role", "fixture-reader", "")
	require.NoError(t, err)
	require.Equal(t, "reader-model", profiled.Model)
	select {
	case request := <-requests:
		require.Equal(t, "reader-model", request.Model)
		require.Len(t, request.Tools, 1)
		require.Equal(t, "view", request.Tools[0].Function.Name)
		require.Contains(t, string(request.Messages), "ROLE_FIXTURE_READ_ONLY")
	case <-time.After(time.Second):
		t.Fatal("named profile did not reach model")
	}
	require.Eventually(t, func() bool {
		for _, task := range c.RuntimeTasks() {
			if task.ID == profiled.ID {
				return task.Status == "completed"
			}
		}
		return false
	}, time.Second, time.Millisecond)
}

func TestNamedSwarmRejectsUnknownOrDisabledAgents(t *testing.T) {
	c := newToolSetTestCoordinator(t)
	parent, err := c.sessions.Create(t.Context(), "validation")
	require.NoError(t, err)
	_, err = c.RuntimeStartSubagent(t.Context(), parent.ID, "Inspect", "does-not-exist", "")
	require.ErrorContains(t, err, "not found")
	definition := c.cfg.Config().Agents[config.AgentTask]
	definition.Disabled = true
	c.cfg.Config().Agents[config.AgentTask] = definition
	_, err = c.RuntimeStartSubagent(t.Context(), parent.ID, "Inspect", config.AgentTask, "")
	require.ErrorContains(t, err, "disabled")
	require.Empty(t, c.RuntimeTasks())
	runtimeTaskStores.Delete(c)
	sc := NewSwarmCoordinator(nil)
	require.Nil(t, sc.resolveAgent("unknown"))
}

func TestSwarmQueuedTaskCancelsWithoutRunning(t *testing.T) {
	c := newToolSetTestCoordinator(t)
	t.Cleanup(func() { c.RuntimeCancelTasks(); runtimeTaskStores.Delete(c) })
	parent, err := c.sessions.Create(t.Context(), "queue")
	require.NoError(t, err)
	first := &runtimeBlockedChild{entered: make(chan struct{})}
	second := &runtimeBlockedChild{entered: make(chan struct{})}
	// The fake child returns the fixture's configured provider identity.
	c.cfg.Config().Providers.Set("unit-only", config.ProviderConfig{})
	a, _ := c.startRuntimeSubagent(t.Context(), subAgentParams{Agent: first, SessionID: parent.ID, AgentMessageID: "m1", ToolCallID: "t1", Prompt: "first"}, true)
	select {
	case <-first.entered:
	case <-time.After(time.Second):
		t.Fatal("first child did not start")
	}
	b, _ := c.startRuntimeSubagent(t.Context(), subAgentParams{Agent: second, SessionID: parent.ID, AgentMessageID: "m2", ToolCallID: "t2", Prompt: "second"}, true)
	require.Eventually(t, func() bool {
		for _, v := range c.RuntimeTasks() {
			if v.ID == b.view.ID {
				return v.Status == "queued" && v.StartedAt == nil
			}
		}
		return false
	}, time.Second, time.Millisecond)
	require.NoError(t, c.RuntimeTaskControl(b.view.ID, "cancel"))
	require.Eventually(t, func() bool {
		for _, v := range c.RuntimeTasks() {
			if v.ID == b.view.ID {
				return v.Status == "cancelled"
			}
		}
		return false
	}, time.Second, time.Millisecond)
	select {
	case <-second.entered:
		t.Fatal("cancelled queued child ran")
	default:
	}
	require.NoError(t, c.RuntimeTaskControl(a.view.ID, "cancel"))
	require.Eventually(t, func() bool { return !c.runtimeChildrenBusy() }, time.Second, time.Millisecond)
}
