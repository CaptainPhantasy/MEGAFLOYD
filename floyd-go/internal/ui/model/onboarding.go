package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/legacy-ai/floyd/internal/home"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

type projectInitializationPromptMsg struct {
	Workspace workspace.Workspace
	SessionID string
	Prompt    string
	Err       error
}

// markProjectInitializedCmd marks the current project as initialized in the config.
func (m *UI) markProjectInitializedCmd() tea.Cmd {
	return func() tea.Msg {
		if err := m.com.Workspace.MarkProjectInitialized(); err != nil {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  fmt.Sprintf("Failed to mark project as initialized: %v", err),
				TTL:  15 * time.Second,
			}
		}
		return nil
	}
}

// updateInitializeView handles keyboard input for the project initialization prompt.
func (m *UI) updateInitializeView(msg tea.KeyPressMsg) (cmds []tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Initialize.Enter):
		if m.onboarding.yesInitializeSelected {
			cmds = append(cmds, m.initializeProject())
		} else {
			cmds = append(cmds, m.skipInitializeProject())
		}
	case key.Matches(msg, m.keyMap.Initialize.Switch):
		m.onboarding.yesInitializeSelected = !m.onboarding.yesInitializeSelected
	case key.Matches(msg, m.keyMap.Initialize.Yes):
		cmds = append(cmds, m.initializeProject())
	case key.Matches(msg, m.keyMap.Initialize.No):
		cmds = append(cmds, m.skipInitializeProject())
	}
	return cmds
}

// initializeProject starts project initialization and transitions to the landing view.
func (m *UI) initializeProject() tea.Cmd {
	ws := m.com.Workspace
	sid := ""
	if m.session != nil {
		sid = m.session.ID
	}
	return func() tea.Msg {
		initPrompt, err := ws.InitializePrompt()
		return projectInitializationPromptMsg{Workspace: ws, SessionID: sid, Prompt: initPrompt, Err: err}
	}
}

func (m *UI) startProjectInitialization(msg projectInitializationPromptMsg) tea.Cmd {
	if msg.Err != nil {
		return util.ReportError(fmt.Errorf("build project initialization: %w", msg.Err))
	}
	if msg.Workspace == m.com.Workspace && (m.session == nil && msg.SessionID == "" || m.session != nil && m.session.ID == msg.SessionID) {
		return m.sendMessage(msg.Prompt)
	}
	// A session switch during prompt construction must not retarget the task.
	return func() tea.Msg {
		sid := msg.SessionID
		if sid == "" {
			created, err := msg.Workspace.CreateSession(context.Background(), "Project initialization")
			if err != nil {
				return builtinResultMsg{Err: err}
			}
			sid = created.ID
		}
		if err := msg.Workspace.AgentRun(context.Background(), sid, msg.Prompt); err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{Info: "Project initialization submitted in session " + sid}
	}
}

// skipInitializeProject skips project initialization and transitions to the landing view.
func (m *UI) skipInitializeProject() tea.Cmd {
	m.setState(uiLanding, uiFocusEditor)
	// Remember the user's explicit dismissal. Starting initialization does not
	// write this flag: normal discovery checks the resulting policy file.
	return m.markProjectInitializedCmd()
}

// initializeView renders the project initialization prompt with Yes/No buttons.
func (m *UI) initializeView() string {
	s := m.com.Styles.Initialize
	cwd := home.Short(m.com.Workspace.WorkingDir())
	initFile := m.com.Config().Options.InitializeAs

	header := s.Header.Render("Would you like to initialize this project?")
	path := s.Accent.PaddingLeft(2).Render(cwd)
	desc := s.Content.Render(fmt.Sprintf("I examine this project, create %s from the bundled standard if it is missing, and record project facts in its SSOT. Existing policy and conversation are preserved.", initFile))
	hint := s.Content.Render("You can also initialize anytime via ") + s.Accent.Render("ctrl+p") + s.Content.Render(".")
	prompt := s.Content.Render("Would you like to initialize now?")

	buttons := common.ButtonGroup(m.com.Styles, []common.ButtonOpts{
		{Text: "Yep!", Selected: m.onboarding.yesInitializeSelected},
		{Text: "Nope", Selected: !m.onboarding.yesInitializeSelected},
	}, " ")

	// max width 60 so the text is compact
	width := min(m.layout.main.Dx(), 60)

	return lipgloss.NewStyle().
		Width(width).
		Height(m.layout.main.Dy()).
		PaddingBottom(1).
		AlignVertical(lipgloss.Bottom).
		Render(strings.Join(
			[]string{
				header,
				path,
				desc,
				hint,
				prompt,
				buttons,
			},
			"\n\n",
		))
}
