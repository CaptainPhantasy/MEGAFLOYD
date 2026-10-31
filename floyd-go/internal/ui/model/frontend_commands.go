package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

func (m *UI) libraryCommand(name, args string) tea.Cmd {
	switch name {
	case "agents":
		return m.openDialog(dialog.AgentLibraryID)
	case "skills":
		return m.openDialog(dialog.SkillsLibraryID)
	case "todos":
		return m.todosCommand()
	case "release-notes":
		return m.releaseNotesCommand()
	case "prompt":
		return m.promptTemplateCommand(args)
	case "file-history":
		return m.showFileHistory(args)
	case "rewind":
		return m.rewindSession(args)
	case "hooks":
		if strings.TrimSpace(args) == "" {
			return m.showInfo("Hooks", "Inspect the effective hook rules without executing them:\n/hooks explain <event> [tool]\n\nRead recorded execution for this session:\n/hooks trace")
		}
	}
	ws, ok := m.com.Workspace.(workspace.FrontendOperations)
	if !ok {
		return util.ReportError(fmt.Errorf("workspace does not expose %s operations", name))
	}
	request := proto.FrontendRequest{Kind: name}
	if m.session != nil {
		request.SessionID = m.session.ID
	}
	action, arg, _ := strings.Cut(strings.TrimSpace(args), " ")
	switch name {
	case "hooks":
		request.Action = action
		request.Argument = strings.TrimSpace(arg)
		if action == "explain" {
			request.Argument, request.ToolName, _ = strings.Cut(strings.TrimSpace(arg), " ")
			request.ToolName = strings.TrimSpace(request.ToolName)
		}
	case "doctor", "memory":
		request.Action = action
	case "codebase":
		request.Action = action
		request.Argument = strings.TrimSpace(arg)
		if action == "" {
			request.Action = "analyze"
		}
	case "telemetry":
		request.Action = action
		request.Argument = strings.TrimSpace(arg)
		if action == "" {
			request.Action = "report"
		}
	case "session":
		request.Action = action
		request.Argument = strings.TrimSpace(arg)
	case "import":
		request.Kind = "session"
		request.Action = "import"
		request.Argument = strings.TrimSpace(args)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		result, err := ws.FrontendOperation(ctx, request)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		body := result.Body
		if result.Path != "" {
			body += "\n\nWorkspace path: " + result.Path
		}
		if result.SessionID != "" && request.Kind == "session" && request.Action == "import" {
			body += "\n\nOpen the imported session with /sessions: " + result.SessionID
		}
		return builtinResultMsg{Title: result.Title, Body: body}
	}
}
