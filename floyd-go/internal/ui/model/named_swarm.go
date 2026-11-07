package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

type namedSwarmUpdateMsg struct {
	Workspace   workspace.Workspace
	SessionID   string
	IDs         []string
	Tasks       []agent.RuntimeTask
	Errors      []string
	LastSummary string
	Err         error
}

func (m *UI) runSwarm(prompt string) tea.Cmd {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return m.promptBuiltinInput("swarm")
	}
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	cfg := m.com.Config()
	if cfg == nil {
		return util.ReportError(fmt.Errorf("workspace configuration is unavailable"))
	}
	if len(cfg.Agents) == 0 {
		return util.ReportWarn("No agents are configured.")
	}
	names := []string{}
	for name, definition := range cfg.Agents {
		if !definition.Disabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return util.ReportWarn("No enabled agents are configured.")
	}
	ws, sid := m.com.Workspace, m.session.ID
	return func() tea.Msg {
		out := namedSwarmUpdateMsg{Workspace: ws, SessionID: sid}
		for _, name := range names {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			task, err := ws.StartAgentTask(ctx, sid, prompt, name)
			cancel()
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			out.IDs = append(out.IDs, task.ID)
			out.Tasks = append(out.Tasks, task)
		}
		return out
	}
}

func pollNamedSwarm(ws workspace.Workspace, previous namedSwarmUpdateMsg) tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tasks, err := ws.Tasks(ctx)
		next := previous
		next.Err = err
		if err == nil {
			next.Tasks = nil
			for _, id := range previous.IDs {
				found := false
				for _, task := range tasks {
					if task.ID == id {
						next.Tasks = append(next.Tasks, task)
						found = true
						break
					}
				}
				if !found {
					next.Err = fmt.Errorf("swarm task %s is no longer available; inspect /tasks", id)
					break
				}
			}
		}
		return next
	})
}

// updateNamedSwarm runs only from UI.Update; asynchronous commands carry data.
func (m *UI) updateNamedSwarm(state namedSwarmUpdateMsg) tea.Cmd {
	// A background task remains in its source workspace after navigation.
	// Stop this view's polling instead of querying the newly selected workspace
	// or placing an unrelated result dialog over its conversation.
	if state.Workspace != nil && (m.com.Workspace != state.Workspace || m.session == nil || m.session.ID != state.SessionID) {
		return nil
	}
	if state.Err != nil {
		return util.ReportError(state.Err)
	}
	pending := 0
	for _, task := range state.Tasks {
		if task.Status == "queued" || task.Status == "running" {
			pending++
		}
	}
	if pending == 0 {
		var body strings.Builder
		for _, task := range state.Tasks {
			fmt.Fprintf(&body, "%s — %s (%s)\nTask: %s\nSession: %s\n%s\n\n", task.SubagentType, task.Status, task.Model, task.ID, task.AgentID, task.OutputPreview)
		}
		for _, err := range state.Errors {
			fmt.Fprintf(&body, "Launch failed: %s\n", err)
		}
		return func() tea.Msg { return builtinResultMsg{Title: "Swarm results", Body: body.String()} }
	}
	parts := []string{}
	for _, task := range state.Tasks {
		parts = append(parts, fmt.Sprintf("%s %s", task.SubagentType, task.Status))
	}
	summary := "Swarm: " + strings.Join(parts, ", ") + ". /tasks shows output and cancel controls."
	previous := state.LastSummary
	state.LastSummary = summary
	ws := state.Workspace
	if ws == nil {
		ws = m.com.Workspace
	}
	poll := pollNamedSwarm(ws, state)
	if summary == previous {
		return poll
	}
	return tea.Batch(util.ReportInfo(summary), poll)
}
