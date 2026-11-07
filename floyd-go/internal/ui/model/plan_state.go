package model

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/message"
)

// planRefreshDelay collapses bursts of agent tool-message events into one
// plan-state read. Streaming can update the same message many times per
// second; only the newest tick is allowed to read the plan file.
const planRefreshDelay = 200 * time.Millisecond

// planRefreshTickMsg asks Update to read plan state for a session. The
// generation guard discards ticks superseded by a newer message.
type planRefreshTickMsg struct {
	SessionID  string
	Generation uint64
}

// planStatusMsg carries durable plan state read off the event loop.
type planStatusMsg struct {
	SessionID string
	Active    bool
	Err       error
}

// requestPlanRefresh schedules a debounced plan-state read for the active
// session. It is safe to call for every tool message.
func (m *UI) requestPlanRefresh() tea.Cmd {
	if m.session == nil {
		return nil
	}
	m.planRefreshGeneration++
	generation := m.planRefreshGeneration
	sessionID := m.session.ID
	return tea.Tick(planRefreshDelay, func(time.Time) tea.Msg {
		return planRefreshTickMsg{SessionID: sessionID, Generation: generation}
	})
}

// planRefreshForMessage schedules a plan-state read when an agent message
// carries tool traffic. enter_plan_mode and exit_plan_mode are tools, so
// this is what makes the pill follow agent-driven plan mode.
func (m *UI) planRefreshForMessage(msg message.Message) tea.Cmd {
	if len(msg.ToolCalls()) == 0 && len(msg.ToolResults()) == 0 {
		return nil
	}
	return m.requestPlanRefresh()
}

// handlePlanRefreshTick accepts the newest debounced tick for the active
// session and ignores ticks superseded by later agent traffic.
func (m *UI) handlePlanRefreshTick(msg planRefreshTickMsg) tea.Cmd {
	if m.session == nil || msg.SessionID != m.session.ID || msg.Generation != m.planRefreshGeneration {
		return nil
	}
	return m.refreshPlanState(msg.SessionID)
}

// refreshPlanState reads the durable plan state for sessionID off the
// event loop. An empty data directory (client/server workspaces without a
// local install) simply yields no command.
func (m *UI) refreshPlanState(sessionID string) tea.Cmd {
	if sessionID == "" || m.com == nil {
		return nil
	}
	ws := m.com.Workspace
	return func() tea.Msg {
		status, err := ws.Plan(context.Background(), sessionID, nil)
		if err != nil {
			return planStatusMsg{SessionID: sessionID, Err: err}
		}
		return planStatusMsg{SessionID: sessionID, Active: status.Active}
	}
}
