package model

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/agent"
)

func (m *UI) tasksCommand(args string) tea.Cmd {
	ws := m.com.Workspace
	return func() tea.Msg {
		fields := strings.Fields(args)
		ctx := context.Background()
		if len(fields) > 0 {
			id, action := fields[0], ""
			if len(fields) == 2 {
				action, id = fields[0], fields[1]
			}
			if len(fields) > 2 || (action != "" && action != "cancel" && action != "detach") {
				return builtinResultMsg{Err: fmt.Errorf("usage: /tasks [id|cancel id|detach id]")}
			}
			item, err := ws.Task(ctx, id, action)
			if err != nil {
				return builtinResultMsg{Err: err}
			}
			return builtinResultMsg{Title: "Task", Body: formatTasks([]agent.RuntimeTask{item})}
		}
		tasks, err := ws.Tasks(ctx)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{Title: "Tasks", Body: formatTasks(tasks)}
	}
}

func formatTasks(tasks []agent.RuntimeTask) string {
	if len(tasks) == 0 {
		return "No background tasks."
	}
	var b strings.Builder
	for _, task := range tasks {
		fmt.Fprintf(&b, "[%s] %s\n%s\nSession: %s\n", task.ID, task.Status, task.Description, task.SessionID)
		if task.OutputPreview != "" {
			fmt.Fprintf(&b, "%s\n", task.OutputPreview)
		}
		b.WriteString("\n")
	}
	b.WriteString("/tasks <id> shows output. /tasks cancel <id> cancels that task.")
	return b.String()
}
