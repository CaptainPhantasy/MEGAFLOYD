package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
)

const WhyToolName = "why"

//go:embed why.md
var whyDescription string

type WhyParams struct {
	JobID    string `json:"job_id" jsonschema:"description=Job contract whose journal holds the action"`
	ActionID string `json:"action_id" jsonschema:"description=Exact journal action id (as returned by another job tool)"`
}

type WhyResponse struct {
	JobID    string `json:"job_id"`
	ActionID string `json:"action_id"`
	Found    bool   `json:"found"`
	Action   string `json:"action,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Result   string `json:"result,omitempty"`
}

// NewWhyTool answers from the journal and reports UNKNOWN instead of
// inventing a rationale.
func NewWhyTool(dataDir, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WhyToolName,
		whyDescription,
		func(ctx context.Context, params WhyParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := jobsDataDirectory(dataDir, workingDir)
			if strings.TrimSpace(params.JobID) == "" {
				return fantasy.NewTextErrorResponse("job_id is required"), nil
			}
			entry, err := jobs.Why(root, params.JobID, params.ActionID)
			if err != nil {
				if errors.Is(err, jobs.ErrUnknownAction) {
					return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
						"UNKNOWN: no journal entry with id %q in job %s. Nothing is inferred.",
						params.ActionID, params.JobID,
					)), WhyResponse{JobID: params.JobID, ActionID: params.ActionID, Found: false}), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "RECORDED: journal entry %s\n", entry.ID)
			fmt.Fprintf(&sb, "Timestamp: %s\n", entry.Timestamp.Format("2006-01-02T15:04:05Z"))
			if entry.Tool != "" {
				fmt.Fprintf(&sb, "Tool: %s\n", entry.Tool)
			}
			fmt.Fprintf(&sb, "Action: %s\n", entry.Action)
			if entry.Requirement != "" {
				fmt.Fprintf(&sb, "Requirement: %s\n", entry.Requirement)
			}
			if entry.Detail != "" {
				fmt.Fprintf(&sb, "Detail: %s\n", entry.Detail)
			}
			if entry.ArtifactHash != "" {
				fmt.Fprintf(&sb, "Artifact SHA-256: %s\n", entry.ArtifactHash)
			}
			if entry.Result != "" {
				fmt.Fprintf(&sb, "Result: %s\n", entry.Result)
			}
			sb.WriteString("INFERRED: none — the fields above are the recorded journal entry.")
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), WhyResponse{
				JobID:    params.JobID,
				ActionID: entry.ID,
				Found:    true,
				Action:   entry.Action,
				Detail:   entry.Detail,
				Result:   entry.Result,
			}), nil
		},
	)
}
