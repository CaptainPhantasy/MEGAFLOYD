package model

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

type namedSwarmWorkspace struct {
	*swarmTestWorkspace
	names   []string
	parents []string
	prompts []string
	tasks   []agent.RuntimeTask
	polls   int
}

func (w *namedSwarmWorkspace) StartAgentTask(_ context.Context, parent, prompt, name string) (agent.RuntimeTask, error) {
	w.names = append(w.names, name)
	w.parents = append(w.parents, parent)
	w.prompts = append(w.prompts, prompt)
	task := agent.RuntimeTask{ID: fmt.Sprint(len(w.names)), AgentID: "child-" + name, SubagentType: name, Model: name + "-model", Status: "queued"}
	w.tasks = append(w.tasks, task)
	return task, nil
}

func (w *namedSwarmWorkspace) Tasks(context.Context) ([]agent.RuntimeTask, error) {
	w.polls++
	return w.tasks, nil
}

func TestNamedSwarmUIStartsActualNamesAndWaitsForQueuedResults(t *testing.T) {
	base := &swarmTestWorkspace{cfg: testConfigWithModel(1000)}
	base.cfg.Agents = map[string]config.Agent{"task": {Model: config.SelectedModelTypeSmall}, "coder": {Model: config.SelectedModelTypeLarge}, "disabled": {Disabled: true}}
	ui := newSwarmTestUI(base)
	ws := &namedSwarmWorkspace{swarmTestWorkspace: base}
	ui.com.Workspace = ws
	msg, ok := ui.runBuiltin("swarm", "run Inspect exact task", false)().(namedSwarmUpdateMsg)
	require.True(t, ok)
	require.Equal(t, []string{"coder", "task"}, ws.names)
	require.Equal(t, []string{"sess-1", "sess-1"}, ws.parents)
	require.Equal(t, []string{"Inspect exact task", "Inspect exact task"}, ws.prompts)
	_, pending := ui.updateNamedSwarm(msg)().(tea.BatchMsg)
	require.True(t, pending, "queued children must produce progress and a future poll, not final results")
	for i := range ws.tasks {
		ws.tasks[i].Status = "completed"
		ws.tasks[i].OutputPreview = "Result for " + ws.tasks[i].SubagentType
	}
	polled := pollNamedSwarm(ws, msg)().(namedSwarmUpdateMsg)
	result := ui.updateNamedSwarm(polled)().(builtinResultMsg)
	require.Contains(t, result.Body, "Result for coder")
	require.Contains(t, result.Body, "Result for task")
	require.Contains(t, result.Body, "child-task")
	require.Contains(t, result.Body, "task-model")
}

func TestNamedSwarmNavigationKeepsTasksInSourceWorkspace(t *testing.T) {
	base := &swarmTestWorkspace{cfg: testConfigWithModel(1000)}
	base.cfg.Agents = map[string]config.Agent{"task": {Model: config.SelectedModelTypeSmall}}
	ui := newSwarmTestUI(base)
	source := &namedSwarmWorkspace{swarmTestWorkspace: base}
	ui.com.Workspace = source
	started := ui.runSwarm("Inspect only")().(namedSwarmUpdateMsg)
	require.Same(t, source, started.Workspace)
	other := &namedSwarmWorkspace{swarmTestWorkspace: base}
	ui.com.Workspace = other
	// An already scheduled poll uses the captured source, never the new UI workspace.
	update := pollNamedSwarm(source, started)().(namedSwarmUpdateMsg)
	require.Equal(t, 1, source.polls)
	require.Zero(t, other.polls)
	require.Nil(t, ui.updateNamedSwarm(update))
	require.Len(t, source.tasks, 1, "navigation must not cancel or lose background work")
	ui.com.Workspace = source
	ui.session = &session.Session{ID: "another-session"}
	update.Tasks[0].Status = "completed"
	require.Nil(t, ui.updateNamedSwarm(update), "a source-session result must not cover another conversation")
}
