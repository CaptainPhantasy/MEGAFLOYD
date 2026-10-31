package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

const swarmUsage = "Usage: /swarm [on|off|status|parallel <1-8>|run <task>]"

// swarmStatusMsg carries a session's durable swarm state back to the
// Update loop, which owns all model mutation.
type swarmStatusMsg struct {
	SessionID string
	State     proto.SwarmState
	Err       error
	// Report is shown as a transient status line.
	Report string
	// Body, when set, opens the read-only info panel.
	Body string
}

// swarmCommand implements /swarm.
//
// Swarm mode controls how many subagents a session may run at once: off
// means strictly one at a time, on means up to MaxParallel. The state is
// stored in session metadata so the agent process and every client see
// the same value.
//
// The bare form and "status" report the current state, "on"/"off" toggle
// it, "parallel N" sets the ceiling, and "run <task>" keeps the older
// behaviour of fanning one task out across every configured agent.
func (m *UI) swarmCommand(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session. Send a prompt first, then use /swarm.")
	}
	if m.com == nil || m.com.Workspace == nil {
		return util.ReportWarn("No workspace is attached.")
	}
	sessionID := m.session.ID
	fields := strings.Fields(args)
	verb := ""
	if len(fields) > 0 {
		verb = strings.ToLower(fields[0])
	}

	switch verb {
	case "", "status":
		return m.refreshSwarmStateWithDetail(sessionID)
	case "on", "off":
		active := verb == "on"
		return m.setSwarm(sessionID, &active, nil)
	case "parallel":
		if len(fields) != 2 {
			return util.ReportWarn("Usage: /swarm parallel <1-8>")
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 || n > 8 {
			return util.ReportWarn("Swarm parallelism must be a number from 1 to 8.")
		}
		return m.setSwarm(sessionID, nil, &n)
	case "run":
		task := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args), fields[0]))
		return m.runSwarm(task)
	default:
		return util.ReportWarn(swarmUsage)
	}
}

// setSwarm writes the swarm state and reports the resulting value.
func (m *UI) setSwarm(sessionID string, active *bool, parallel *int) tea.Cmd {
	ws := m.com.Workspace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		st, err := ws.SetSwarm(ctx, sessionID, active, parallel)
		if err != nil {
			return swarmStatusMsg{SessionID: sessionID, Err: fmt.Errorf("failed to update swarm mode: %w", err)}
		}
		report := ""
		switch {
		case active != nil && *active:
			report = fmt.Sprintf("Swarm mode on: up to %d subagents run at once.", st.MaxParallel)
		case active != nil:
			report = "Swarm mode off: subagents run one at a time."
		case parallel != nil:
			if st.Active {
				report = fmt.Sprintf("Swarm parallelism set to %d.", st.MaxParallel)
			} else {
				report = fmt.Sprintf("Swarm parallelism set to %d; it applies when swarm mode is on.", st.MaxParallel)
			}
		}
		return swarmStatusMsg{SessionID: sessionID, State: st, Report: report}
	}
}

// refreshSwarmState reads the swarm state and keeps the status pill in
// sync without opening a panel.
func (m *UI) refreshSwarmState(sessionID string) tea.Cmd {
	if sessionID == "" || m.com == nil || m.com.Workspace == nil {
		return nil
	}
	ws := m.com.Workspace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		st, err := ws.Swarm(ctx, sessionID)
		return swarmStatusMsg{SessionID: sessionID, State: st, Err: err}
	}
}

// refreshSwarmStateWithDetail reads the swarm state and shows it in the
// info panel, for /swarm and /swarm status.
func (m *UI) refreshSwarmStateWithDetail(sessionID string) tea.Cmd {
	if m.com == nil || m.com.Workspace == nil {
		return util.ReportWarn("No workspace is attached.")
	}
	ws := m.com.Workspace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		st, err := ws.Swarm(ctx, sessionID)
		if err != nil {
			return swarmStatusMsg{SessionID: sessionID, Err: fmt.Errorf("failed to read swarm mode: %w", err)}
		}
		mode := "off"
		if st.Active {
			mode = "on"
		}
		body := fmt.Sprintf(
			"Swarm mode: %s\nSubagent parallelism: %d\n\nWhen swarm mode is on, a session may run up to %d subagents at once. When it is off, subagents run one at a time.\n\n/swarm on|off, /swarm parallel <1-8>, /swarm run <task>",
			mode, st.MaxParallel, st.MaxParallel,
		)
		return swarmStatusMsg{SessionID: sessionID, State: st, Body: body}
	}
}
