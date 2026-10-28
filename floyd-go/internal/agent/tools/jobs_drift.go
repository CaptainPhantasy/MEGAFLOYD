package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/jobs"
)

const DriftToolName = "drift"

//go:embed drift.md
var driftDescription string

type DriftParams struct {
	JobID string   `json:"job_id" jsonschema:"description=Job contract whose declared scope is the reference"`
	Paths []string `json:"paths,omitempty" jsonschema:"description=Optional explicit changed paths; defaults to git status in the working directory"`
}

type DriftResponse struct {
	JobID      string   `json:"job_id"`
	Scope      []string `json:"scope,omitempty"`
	Changed    int      `json:"changed"`
	Violations int      `json:"violations"`
}

// NewDriftTool compares changed files against the contract's declared scope.
func NewDriftTool(dataDir, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DriftToolName,
		driftDescription,
		func(ctx context.Context, params DriftParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root, err := jobsDataDirectory(dataDir, workingDir)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if strings.TrimSpace(params.JobID) == "" {
				return fantasy.NewTextErrorResponse("job_id is required"), nil
			}
			job, err := jobs.Load(root, params.JobID)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			changed := params.Paths
			if len(changed) == 0 {
				changed, err = gitChangedPaths(ctx, workingDir)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
			}
			violations, err := jobs.CheckDrift(root, job.ID, changed)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "DRIFT REPORT %s\n", job.ID)
			fmt.Fprintf(&sb, "Declared scope: %s\n", scopeList(job.Scope))
			fmt.Fprintf(&sb, "Changed paths: %d\n", len(changed))
			if len(violations) == 0 {
				sb.WriteString("No drift: every changed path is inside the declared scope.")
				return fantasy.NewTextResponse(sb.String()), nil
			}
			sb.WriteString("Violations:\n")
			for _, violation := range violations {
				fmt.Fprintf(&sb, "- %s — %s\n", violation.Path, violation.Reason)
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(strings.TrimRight(sb.String(), "\n")), DriftResponse{
				JobID:      job.ID,
				Scope:      job.Scope,
				Changed:    len(changed),
				Violations: len(violations),
			}), nil
		},
	)
}

func gitChangedPaths(ctx context.Context, workingDir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading git status in %s: %w (pass paths explicitly if this is not a git repository)", workingDir, err)
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if _, after, ok := strings.Cut(path, " -> "); ok {
			path = after
		}
		if strings.HasPrefix(path, `"`) && strings.HasSuffix(path, `"`) {
			if unquoted, err := strconv.Unquote(path); err == nil {
				path = unquoted
			}
		}
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func scopeList(scope []string) string {
	if len(scope) == 0 {
		return "(none declared — every change is a violation)"
	}
	return strings.Join(scope, ", ")
}
