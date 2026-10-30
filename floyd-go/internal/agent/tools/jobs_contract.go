package tools

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/legacy-ai/floyd/internal/permission"
)

const ContractToolName = "contract"

//go:embed contract.md
var contractDescription string

type ContractParams struct {
	Action       string   `json:"action" jsonschema:"description=Action to perform,enum=create,enum=list,enum=show,enum=complete"`
	JobID        string   `json:"job_id,omitempty" jsonschema:"description=Job id (returned by create)"`
	Objective    string   `json:"objective,omitempty" jsonschema:"description=One-sentence objective, required for create"`
	Requirements []string `json:"requirements,omitempty" jsonschema:"description=Checkable outcomes; completion needs passing evidence for each"`
	Scope        []string `json:"scope,omitempty" jsonschema:"description=Paths this job may change; drift reports anything outside"`
	Artifact     string   `json:"artifact,omitempty" jsonschema:"description=Artifact path whose SHA-256 binds evidence across the job"`
}

type ContractResponse struct {
	JobID        string   `json:"job_id,omitempty"`
	Path         string   `json:"path,omitempty"`
	Status       string   `json:"status,omitempty"`
	Complete     bool     `json:"complete"`
	Requirements []string `json:"requirements,omitempty"`
	Blocking     []string `json:"blocking,omitempty"`
}

// NewContractTool creates, lists, shows, and completes job contracts.
func NewContractTool(dataDir string, permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ContractToolName,
		contractDescription,
		func(ctx context.Context, params ContractParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root, err := jobsDataDirectory(dataDir, workingDir)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			switch strings.ToLower(strings.TrimSpace(params.Action)) {
			case "create":
				return contractCreate(ctx, root, workingDir, permissions, call, params)
			case "list":
				return contractList(root)
			case "show":
				return contractShow(root, params.JobID)
			case "complete":
				return contractComplete(ctx, root, permissions, call, params.JobID)
			default:
				return fantasy.NewTextErrorResponse("action must be one of: create, list, show, complete"), nil
			}
		},
	)
}

func contractCreate(
	ctx context.Context,
	root, workingDir string,
	permissions permission.Service,
	call fantasy.ToolCall,
	params ContractParams,
) (fantasy.ToolResponse, error) {
	artifact := strings.TrimSpace(params.Artifact)
	if artifact != "" && !filepath.IsAbs(artifact) {
		artifact = filepath.Join(workingDir, artifact)
	}
	granted, err := requestJobsPermission(
		ctx, permissions, call, ContractToolName, "create",
		filepath.Join(root, "jobs"),
		"Create job contract: "+params.Objective,
		params,
	)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if !granted {
		return NewPermissionDeniedResponse(), nil
	}

	job, err := jobs.Create(root, jobs.CreateOptions{
		Objective:    params.Objective,
		Requirements: params.Requirements,
		Artifact:     artifact,
		Scope:        params.Scope,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	var lines []string
	for _, req := range job.Requirements {
		lines = append(lines, fmt.Sprintf("- %s: %s", req.ID, req.Text))
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
		"Contract created: %s\nPath: %s\nRequirements:\n%s",
		job.ID, filepath.Join(root, "jobs", job.ID), strings.Join(lines, "\n"),
	)), ContractResponse{
		JobID:        job.ID,
		Path:         filepath.Join(root, "jobs", job.ID),
		Status:       job.Status,
		Requirements: requirementIDs(job.Requirements),
	}), nil
}

func contractList(root string) (fantasy.ToolResponse, error) {
	list, err := jobs.List(root)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(list) == 0 {
		return fantasy.NewTextResponse("No job contracts recorded."), nil
	}

	var sb strings.Builder
	sb.WriteString("JOB CONTRACTS\n")
	for _, job := range list {
		status, err := jobs.Status(root, job.ID)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		passed := 0
		for _, req := range status.Requirements {
			if req.Status == jobs.VerdictPassed {
				passed++
			}
		}
		fmt.Fprintf(&sb, "- %s [%s] %d/%d passed — %s\n",
			job.ID, job.Status, passed, len(job.Requirements), job.Objective)
	}
	return fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), nil
}

func contractShow(root, jobID string) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(jobID) == "" {
		return fantasy.NewTextErrorResponse("job_id is required"), nil
	}
	job, err := jobs.Load(root, jobID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	status, err := jobs.Status(root, job.ID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "JOB %s\n", job.ID)
	fmt.Fprintf(&sb, "Objective: %s\n", job.Objective)
	fmt.Fprintf(&sb, "Status: %s\n", job.Status)
	if job.Artifact != "" {
		fmt.Fprintf(&sb, "Artifact: %s\n", job.Artifact)
		fmt.Fprintf(&sb, "Current artifact SHA-256: %s\n", orJobNone(status.CurrentArtifactHash))
		fmt.Fprintf(&sb, "Created with artifact SHA-256: %s\n", orJobNone(job.ArtifactHash))
	}
	if len(job.Scope) > 0 {
		fmt.Fprintf(&sb, "Scope: %s\n", strings.Join(job.Scope, ", "))
	}
	sb.WriteString("Requirements:\n")
	for _, req := range status.Requirements {
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", req.Status, req.ID, req.Text)
		if req.Reason != "" {
			fmt.Fprintf(&sb, "  reason: %s\n", req.Reason)
		}
		for i, ev := range req.Evidence {
			fmt.Fprintf(&sb, "  evidence %d: exit %d, artifact %s, %s, command: %s\n",
				i+1, ev.ExitCode, shortJobHash(ev.ArtifactHash), ev.Timestamp.Format("2006-01-02T15:04:05Z"), ev.Command)
		}
	}
	fmt.Fprintf(&sb, "Complete: %t\n", status.Complete)
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), ContractResponse{
		JobID:        job.ID,
		Path:         filepath.Join(root, "jobs", job.ID),
		Status:       job.Status,
		Complete:     status.Complete,
		Requirements: requirementIDs(job.Requirements),
	}), nil
}

func contractComplete(
	ctx context.Context,
	root string,
	permissions permission.Service,
	call fantasy.ToolCall,
	jobID string,
) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(jobID) == "" {
		return fantasy.NewTextErrorResponse("job_id is required"), nil
	}
	report, err := jobs.CheckCompletion(root, jobID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if !report.Complete {
		return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
			"Completion refused: %d requirement(s) lack passing evidence for the current artifact.\n%s",
			len(report.Blocking), strings.Join(report.Blocking, "\n"),
		)), ContractResponse{JobID: jobID, Complete: false, Blocking: report.Blocking}), nil
	}

	granted, err := requestJobsPermission(
		ctx, permissions, call, ContractToolName, "write",
		filepath.Join(root, "jobs", jobID),
		"Mark job contract complete: "+jobID,
		ContractParams{JobID: jobID},
	)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if !granted {
		return NewPermissionDeniedResponse(), nil
	}

	job, err := jobs.MarkComplete(root, jobID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
		"Job %s marked complete. Every requirement has passing evidence for artifact %s.",
		job.ID, shortJobHash(report.ArtifactHash),
	)), ContractResponse{JobID: job.ID, Status: job.Status, Complete: true}), nil
}

func requirementIDs(requirements []jobs.Requirement) []string {
	out := make([]string, 0, len(requirements))
	for _, req := range requirements {
		out = append(out, req.ID)
	}
	return out
}

func orJobNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
