package model

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/ui/terminal"
	"github.com/legacy-ai/floyd/internal/ui/util"
)

// terminalReadyMsg carries a started terminal component back to Update.
// The component owns the PTY, so it must be created off the event loop.
type terminalReadyMsg struct {
	Component *terminal.Component
	Cmd       tea.Cmd
	Err       error
}

// startTerminal spawns the shell at the current main-pane size. The
// returned command is the first PTY read; terminalReadyMsg hands it back
// to Update so the read loop starts inside Bubble Tea.
func (m *UI) startTerminal() tea.Cmd {
	cols, rows := m.terminalSize()
	cwd := ""
	if m.com != nil && m.com.Workspace != nil {
		cwd = m.com.Workspace.WorkingDir()
	}
	return func() tea.Msg {
		component, cmd, err := terminal.New("main", rows, cols, cwd)
		if err != nil {
			return terminalReadyMsg{Err: fmt.Errorf("start terminal: %w", err)}
		}
		return terminalReadyMsg{Component: component, Cmd: cmd}
	}
}

// terminalSize returns the drawable size of the main pane, clamped so a
// very small terminal cannot ask the PTY for a zero-row window.
func (m *UI) terminalSize() (cols, rows int) {
	cols = max(20, m.layout.main.Dx())
	rows = max(5, m.layout.main.Dy())
	return cols, rows
}

// resizeTerminalCmd resizes the PTY off the event loop after a window
// resize.
func (m *UI) resizeTerminalCmd() tea.Cmd {
	if m.terminal == nil {
		return nil
	}
	component := m.terminal
	cols, rows := m.terminalSize()
	return func() tea.Msg {
		component.Resize(cols, rows)
		return nil
	}
}

// toggleTerminal shows or hides the embedded shell. The first open starts
// a session; later opens reuse it. A shell that exited is replaced.
func (m *UI) toggleTerminal() tea.Cmd {
	if m.terminalVisible {
		m.terminalVisible = false
		if m.terminal != nil {
			m.terminal.Blur()
		}
		m.focus = uiFocusEditor
		m.textarea.Focus()
		return nil
	}

	m.terminalVisible = true
	if m.terminal != nil && m.terminal.Active() {
		m.terminal.Focus()
		return m.resizeTerminalCmd()
	}
	if m.terminal != nil {
		m.terminal.Close()
		m.terminal = nil
	}
	return m.startTerminal()
}

// updateTerminal forwards a message to the embedded terminal. It returns
// nil when the terminal did not handle the message.
func (m *UI) updateTerminal(msg tea.Msg) tea.Cmd {
	if m.terminal == nil {
		return nil
	}
	if _, ok := msg.(terminal.ClosedMsg); ok {
		m.terminalVisible = false
		m.focus = uiFocusEditor
		return util.ReportInfo("Terminal exited. Press alt+t to start a new shell.")
	}
	handled, cmd := m.terminal.Update(msg)
	if !handled {
		return nil
	}
	return cmd
}

// closeTerminal tears down the PTY. Called before the app exits so the
// shell process does not outlive the TUI.
func (m *UI) closeTerminal() {
	if m.terminal == nil {
		return
	}
	m.terminal.Close()
	m.terminal = nil
	m.terminalVisible = false
}
