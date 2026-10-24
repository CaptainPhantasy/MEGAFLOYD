package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
)

//go:embed apply_unified_diff.md
var applyUnifiedDiffDescription []byte

// lookupBinary checks if a binary exists in PATH and returns its path or an error
func lookupBinary(name string) (string, error) {
	return exec.LookPath(name)
}

type ApplyPatchParams struct {
	Patch string `json:"patch" jsonschema:"description=The unified diff block to apply (must be standard a/b diff format)"`
}

func NewApplyPatchTool(permissions permission.Service, wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"apply_patch",
		string(applyUnifiedDiffDescription),
		func(ctx context.Context, params ApplyPatchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if pathErr := checkToolPath(permissions, wd); pathErr != nil {
				return deniedPathResponse(pathErr), nil
			}
			for _, target := range patchTargetPaths(params.Patch) {
				if pathErr := checkToolPath(permissions, resolvePatchPath(wd, target)); pathErr != nil {
					return deniedPathResponse(pathErr), nil
				}
			}

			// A unified diff can touch any file under the working
			// directory, so the permission request covers the whole
			// tree rather than a single path.
			granted, err := permissions.Request(
				ctx,
				permission.CreatePermissionRequest{
					SessionID:   GetSessionFromContext(ctx),
					Path:        wd,
					ToolCallID:  call.ID,
					ToolName:    "apply_patch",
					Action:      "write",
					Description: "Apply a unified diff patch to the working directory",
					Params:      params,
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			// Check if patch binary exists
			patchBin, err := lookupBinary("patch")
			if err != nil {
				return fantasy.NewTextErrorResponse("patch binary not found in $PATH. Please install GNU patch or apply the diff manually."), nil
			}

			// Feed the approved patch directly; no project-shared scratch file.
			cmd := exec.CommandContext(ctx, patchBin, "-p1")
			cmd.Stdin = strings.NewReader(params.Patch)
			cmd.Dir = wd
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fantasy.NewTextResponse(fmt.Sprintf("Failed to apply patch: %s\nOutput: %s", err, string(out))), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Patch applied successfully:\n%s", string(out))), nil
		},
	)
}
