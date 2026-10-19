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

const ProveToolName = "prove"

//go:embed prove.md
var proveDescription string

type ProveParams struct {
	Action          string `json:"action" jsonschema:"description=Action to perform,enum=record,enum=show"`
	JobID           string `json:"job_id" jsonschema:"description=Job contract id"`
	Requirement     string `json:"requirement,omitempty" jsonschema:"description=Requirement id the evidence supports (record), or filter (show)"`
	Command         string `json:"command,omitempty" jsonschema:"description=Exact command or check that produced the evidence"`
	Output          string `json:"output,omitempty" jsonschema:"description=Decisive output lines; stored as an excerpt with a SHA-256"`
	ExitCode        int    `json:"exit_code,omitempty" jsonschema:"description=Exit code of the check; 0 means passed"`
	Artifact        string `json:"artifact,omitempty" jsonschema:"description=Artifact path to hash; defaults to the job artifact"`
	ArtifactHash    string `json:"artifact_hash,omitempty" jsonschema:"description=Pre-computed artifact SHA-256; overrides artifact hashing"`
	ArtifactVersion string `json:"artifact_version,omitempty" jsonschema:"description=Optional version label for the artifact"`
}

type ProveResponse struct {
	JobID        string `json:"job_id"`
	Requirement  string `json:"requirement"`
	Passed       bool   `json:"passed"`
	ArtifactHash string `json:"artifact_hash"`
	EvidencePath string `json:"evidence_path,omitempty"`
}

// NewProveTool records evidence bound to an artifact hash and shows it.
func NewProveTool(dataDir string, permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ProveToolName,
		proveDescription,
		func(ctx context.Context, params ProveParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := jobsDataDirectory(dataDir, workingDir)
			switch strings.ToLower(strings.TrimSpace(params.Action)) {
			case "record":
				return proveRecord(ctx, root, workingDir, permissions, call, params)
			case "show":
				return proveShow(root, params.JobID, params.Requirement)
			default:
				return fantasy.NewTextErrorResponse("action must be one of: record, show"), nil
			}
		},
	)
}

func proveRecord(
	ctx context.Context,
	root, workingDir string,
	permissions permission.Service,
	call fantasy.ToolCall,
	params ProveParams,
) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.JobID) == "" {
		return fantasy.NewTextErrorResponse("job_id is required"), nil
	}
	if strings.TrimSpace(params.Requirement) == "" {
		return fantasy.NewTextErrorResponse("requirement is required"), nil
	}
	if strings.TrimSpace(params.Command) == "" {
		return fantasy.NewTextErrorResponse("command is required"), nil
	}
	job, err := jobs.Load(root, params.JobID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	hash, err := artifactHashFor(workingDir, params.Artifact, params.ArtifactHash, job.Artifact)
	if err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"evidence must be bound to an artifact hash: %v (pass artifact or artifact_hash, or declare an artifact at contract create)", err,
		)), nil
	}

	granted, err := requestJobsPermission(
		ctx, permissions, call, ProveToolName, "write",
		filepath.Join(root, "jobs", job.ID),
		"Record evidence for "+job.ID+"/"+params.Requirement,
		params,
	)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if !granted {
		return NewPermissionDeniedResponse(), nil
	}

	ev, err := jobs.RecordEvidence(root, job.ID, jobs.EvidenceInput{
		Requirement:     params.Requirement,
		Command:         params.Command,
		ExitCode:        params.ExitCode,
		Output:          params.Output,
		ArtifactHash:    hash,
		ArtifactVersion: params.ArtifactVersion,
	})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	verdict := "FAILED"
	if ev.Passed {
		verdict = "PASSED"
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
		"Evidence recorded: %s / %s — %s\nArtifact SHA-256: %s\nOutput SHA-256: %s\nExcerpt: %s/%s\nExit code: %d",
		job.ID, ev.Requirement, verdict,
		ev.ArtifactHash, ev.OutputHash,
		filepath.Join(root, "jobs", job.ID), ev.ExcerptPath, ev.ExitCode,
	)), ProveResponse{
		JobID:        job.ID,
		Requirement:  ev.Requirement,
		Passed:       ev.Passed,
		ArtifactHash: ev.ArtifactHash,
		EvidencePath: filepath.Join(root, "jobs", job.ID, ev.ExcerptPath),
	}), nil
}

func proveShow(root, jobID, requirement string) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(jobID) == "" {
		return fantasy.NewTextErrorResponse("job_id is required"), nil
	}
	status, err := jobs.Status(root, jobID)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "EVIDENCE FOR %s\n", status.JobID)
	if status.Artifact != "" {
		fmt.Fprintf(&sb, "Current artifact SHA-256: %s\n", orJobNone(status.CurrentArtifactHash))
		if status.ArtifactError != "" {
			fmt.Fprintf(&sb, "Artifact error: %s\n", status.ArtifactError)
		}
	}
	found := false
	for _, req := range status.Requirements {
		if requirement != "" && req.ID != requirement {
			continue
		}
		found = true
		fmt.Fprintf(&sb, "- [%s] %s: %s\n", req.Status, req.ID, req.Text)
		if req.Reason != "" {
			fmt.Fprintf(&sb, "  reason: %s\n", req.Reason)
		}
		for i, ev := range req.Evidence {
			fmt.Fprintf(&sb, "  %d. exit %d, artifact %s, output %s, %s\n     command: %s\n     excerpt: %s\n",
				i+1, ev.ExitCode, shortJobHash(ev.ArtifactHash), shortJobHash(ev.OutputHash),
				ev.Timestamp.Format("2006-01-02T15:04:05Z"), ev.Command,
				filepath.Join(root, "jobs", status.JobID, ev.ExcerptPath))
		}
	}
	if !found {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("requirement %q not found in job %s", requirement, jobID)), nil
	}
	return fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), nil
}
