package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
)

const RecoverToolName = "recover"

//go:embed recover.md
var recoverDescription string

type RecoverParams struct {
	JobID string `json:"job_id,omitempty" jsonschema:"description=Specific job to reconstruct; omit to summarize every unfinished job"`
}

type RecoverResponse struct {
	Jobs []RecoverJob `json:"jobs"`
}

type RecoverJob struct {
	JobID        string   `json:"job_id"`
	Objective    string   `json:"objective"`
	Status       string   `json:"status"`
	Complete     bool     `json:"complete"`
	NextAction   string   `json:"next_action"`
	LastJournal  []string `json:"last_journal,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// NewRecoverTool reconstructs unfinished work from durable records.
func NewRecoverTool(dataDir, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		RecoverToolName,
		recoverDescription,
		func(ctx context.Context, params RecoverParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root, err := jobsDataDirectory(dataDir, workingDir)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if strings.TrimSpace(params.JobID) != "" {
				report, text, err := recoverOne(root, params.JobID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				return fantasy.WithResponseMetadata(fantasy.NewTextResponse(text), RecoverResponse{
					Jobs: []RecoverJob{report},
				}), nil
			}

			list, err := jobs.List(root)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			var reports []RecoverJob
			var sb strings.Builder
			for _, job := range list {
				if job.Status == jobs.StatusComplete {
					continue
				}
				report, text, err := recoverOne(root, job.ID)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				reports = append(reports, report)
				sb.WriteString(text)
				sb.WriteString("\n\n")
			}
			if len(reports) == 0 {
				return fantasy.WithResponseMetadata(fantasy.NewTextResponse("No unfinished jobs recorded."), RecoverResponse{}), nil
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), RecoverResponse{
				Jobs: reports,
			}), nil
		},
	)
}

func recoverOne(root, jobID string) (RecoverJob, string, error) {
	job, err := jobs.Load(root, jobID)
	if err != nil {
		return RecoverJob{}, "", err
	}
	status, err := jobs.Status(root, job.ID)
	if err != nil {
		return RecoverJob{}, "", err
	}
	entries, err := jobs.JournalEntries(root, job.ID, 3)
	if err != nil {
		return RecoverJob{}, "", err
	}

	report := RecoverJob{
		JobID:      job.ID,
		Objective:  job.Objective,
		Status:     job.Status,
		Complete:   status.Complete,
		NextAction: recoverNextAction(status),
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "JOB %s\nObjective: %s\nStatus: %s\nNext action: %s\n",
		job.ID, job.Objective, job.Status, report.NextAction)
	for _, req := range status.Requirements {
		report.Requirements = append(report.Requirements, req.Status+":"+req.ID)
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", req.Status, req.ID, req.Text)
		if req.Reason != "" {
			fmt.Fprintf(&sb, "  reason: %s\n", req.Reason)
		}
	}
	if len(entries) == 0 {
		sb.WriteString("Journal: empty\n")
	} else {
		sb.WriteString("Last journal entries:\n")
		for _, entry := range entries {
			line := fmt.Sprintf("%s %s: %s", entry.ID, entry.Tool, entry.Action)
			if entry.Result != "" {
				line += " => " + entry.Result
			}
			report.LastJournal = append(report.LastJournal, line)
			fmt.Fprintf(&sb, "- %s\n", line)
		}
	}
	return report, strings.TrimRight(sb.String(), "\n"), nil
}

func recoverNextAction(status *jobs.JobStatus) string {
	for _, req := range status.Requirements {
		if req.Status != jobs.VerdictPassed {
			if req.Reason != "" {
				return fmt.Sprintf("Produce passing evidence for %s (%s)", req.ID, req.Reason)
			}
			return "Produce passing evidence for " + req.ID
		}
	}
	return "Mark the contract complete"
}
