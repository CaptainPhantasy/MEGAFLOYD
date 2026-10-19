package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"

	"charm.land/fantasy"
)

//go:embed get_active_diff.md
var getActiveDiffDescription []byte

type GetActiveDiffParams struct {
	StagedOnly bool `json:"staged_only" jsonschema:"description=If true, only show staged changes"`
}

func NewGetActiveDiffTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"get_active_diff",
		string(getActiveDiffDescription),
		func(ctx context.Context, params GetActiveDiffParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Validate git binary exists in PATH
			if _, err := lookupBinary("git"); err != nil {
				return fantasy.NewTextResponse("git binary not found in PATH. Please ensure git is installed."), nil
			}

			args := []string{"diff"}
			if params.StagedOnly {
				args = append(args, "--staged")
			}

			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = wd
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fantasy.NewTextResponse(fmt.Sprintf("Failed to get diff: %s\n%s", err, string(out))), nil
			}

			if len(out) == 0 {
				return fantasy.NewTextResponse("No changes found."), nil
			}

			return fantasy.NewTextResponse(string(out)), nil
		},
	)
}
