package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
	"github.com/legacy-ai/floyd/internal/permission"
)

const HandoffToolName = "handoff"

//go:embed handoff.md
var handoffDescription string

type HandoffParams struct {
	JobID string `json:"job_id" jsonschema:"description=Job contract to package for the next session"`
}

type HandoffResponse struct {
	JobID string `json:"job_id"`
	Path  string `json:"path"`
}

// NewHandoffTool writes the portable handoff packet for a job.
func NewHandoffTool(dataDir string, permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		HandoffToolName,
		handoffDescription,
		func(ctx context.Context, params HandoffParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := jobsDataDirectory(dataDir, workingDir)
			if strings.TrimSpace(params.JobID) == "" {
				return fantasy.NewTextErrorResponse("job_id is required"), nil
			}
			granted, err := requestJobsPermission(
				ctx, permissions, call, HandoffToolName, "write",
				filepath.Join(root, "jobs", params.JobID),
				"Write handoff for "+params.JobID,
				params,
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			path, err := jobs.ExportHandoff(root, params.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("handoff written to %s but could not be read back: %v", path, err)), nil
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(fmt.Sprintf(
				"Handoff written: %s\n\n%s", path, strings.TrimRight(string(content), "\n"),
			)), HandoffResponse{JobID: params.JobID, Path: path}), nil
		},
	)
}
