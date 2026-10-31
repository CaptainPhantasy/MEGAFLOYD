package model

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
)

func (m *UI) showFileHistory(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	ws, sid := m.com.Workspace, m.session.ID
	versionID := strings.TrimSpace(args)
	return func() tea.Msg {
		files, err := ws.ListSessionHistory(context.Background(), sid)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		body, err := fileHistoryReport(files, versionID)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		return builtinResultMsg{Title: "File history", Body: body}
	}
}

func fileHistoryReport(files []history.File, id string) (string, error) {
	if id != "" {
		for _, file := range files {
			if file.ID == id {
				return fmt.Sprintf("%s — version %d\n\n%s", file.Path, file.Version, file.Content), nil
			}
		}
		return "", fmt.Errorf("file version %q was not found in this session; use /file-history to list versions", id)
	}
	if len(files) == 0 {
		return "No file versions have been recorded in this session.", nil
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}
		if files[i].Version != files[j].Version {
			return files[i].Version > files[j].Version
		}
		return files[i].ID < files[j].ID
	})
	var out strings.Builder
	out.WriteString("Recorded native file edits. Use /file-history <version-id> to read the complete content.\n\n")
	for _, file := range files {
		fmt.Fprintf(&out, "%s\n  version %d · %s\n", file.Path, file.Version, file.ID)
	}
	return out.String(), nil
}

func (m *UI) rewindSession(args string) tea.Cmd {
	if m.session == nil {
		return util.ReportWarn("No active session.")
	}
	if m.isAgentBusy() {
		return util.ReportWarn("Wait for the active task to finish before rewinding.")
	}
	count := 1
	if value := strings.TrimSpace(args); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return util.ReportWarn("Usage: /rewind [number of recent turns]")
		}
		count = n
	}
	ws, sid := m.com.Workspace, m.session.ID
	rewinder, ok := ws.(workspace.FileRewindWorkspace)
	if !ok {
		return util.ReportWarn("File checkpoint restoration is unavailable in this workspace.")
	}
	return func() tea.Msg {
		result, err := rewinder.RewindSession(context.Background(), sid, count)
		if err != nil {
			return builtinResultMsg{Err: err}
		}
		messages, err := ws.ListMessages(context.Background(), sid)
		if err != nil {
			return builtinResultMsg{Err: fmt.Errorf("rewind completed but transcript refresh failed: %w", err)}
		}
		info := fmt.Sprintf("Rewound %d turn(s) and %d recorded native file(s). Transcript recovery: %s", count, len(result.RestoredFiles), result.Transcript.RecordID)
		if result.RecoveryPath != "" {
			info += "\nPrevious files retained at " + result.RecoveryPath
		}
		return builtinResultMsg{SessionID: sid, Messages: messages, RefreshMessages: true, Info: info}
	}
}
