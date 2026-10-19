package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
)

const ReplayToolName = "replay"

//go:embed replay.md
var replayDescription string

type ReplayParams struct {
	JobID       string `json:"job_id" jsonschema:"description=Job contract whose recorded actions and evidence are listed"`
	Requirement string `json:"requirement,omitempty" jsonschema:"description=Filter evidence to one requirement"`
}

type ReplayResponse struct {
	JobID      string   `json:"job_id"`
	Journal    []string `json:"journal,omitempty"`
	Reexecuted bool     `json:"reexecuted"`
}

// NewReplayTool lists the recorded journal and evidence. It never re-runs a
// mutation.
func NewReplayTool(dataDir, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ReplayToolName,
		replayDescription,
		func(ctx context.Context, params ReplayParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := jobsDataDirectory(dataDir, workingDir)
			if strings.TrimSpace(params.JobID) == "" {
				return fantasy.NewTextErrorResponse("job_id is required"), nil
			}
			status, err := jobs.Status(root, params.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			entries, err := jobs.JournalEntries(root, params.JobID, 0)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "REPLAY VIEW %s (read-only)\n", status.JobID)
			sb.WriteString("No commands are executed by this tool.\n\nJournal:\n")
			journal := make([]string, 0, len(entries))
			for _, entry := range entries {
				line := fmt.Sprintf("%s %s: %s", entry.ID, entry.Tool, entry.Action)
				if entry.Detail != "" {
					line += " (" + entry.Detail + ")"
				}
				if entry.Result != "" {
					line += " => " + entry.Result
				}
				journal = append(journal, line)
				fmt.Fprintf(&sb, "- %s\n", line)
			}
			if len(entries) == 0 {
				sb.WriteString("- (empty)\n")
			}

			sb.WriteString("\nEvidence:\n")
			for _, req := range status.Requirements {
				if params.Requirement != "" && req.ID != params.Requirement {
					continue
				}
				fmt.Fprintf(&sb, "- [%s] %s\n", req.Status, req.ID)
				for i, ev := range req.Evidence {
					fmt.Fprintf(&sb, "  %d. command: %s\n     exit: %d, artifact: %s, output: %s\n     excerpt: %s\n",
						i+1, ev.Command, ev.ExitCode, shortJobHash(ev.ArtifactHash), shortJobHash(ev.OutputHash), ev.ExcerptPath)
				}
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), ReplayResponse{
				JobID:      status.JobID,
				Journal:    journal,
				Reexecuted: false,
			}), nil
		},
	)
}
