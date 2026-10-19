package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"charm.land/fantasy"
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

func NewApplyPatchTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"apply_patch",
		string(applyUnifiedDiffDescription),
		func(ctx context.Context, params ApplyPatchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Check if patch binary exists
			patchBin, err := lookupBinary("patch")
			if err != nil {
				return fantasy.NewTextErrorResponse("patch binary not found in $PATH. Please install GNU patch or apply the diff manually."), nil
			}

			patchFile := filepath.Join(wd, ".floyd", "temp.patch")
			os.MkdirAll(filepath.Dir(patchFile), 0755)
			
			if err := os.WriteFile(patchFile, []byte(params.Patch), 0644); err != nil {
				return fantasy.NewTextResponse(err.Error()), nil
			}
			defer os.Remove(patchFile)

			cmd := exec.CommandContext(ctx, patchBin, "-p1", "-i", patchFile)
			cmd.Dir = wd
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fantasy.NewTextResponse(fmt.Sprintf("Failed to apply patch: %s\nOutput: %s", err, string(out))), nil
			}

			return fantasy.NewTextResponse(fmt.Sprintf("Patch applied successfully:\n%s", string(out))), nil
		},
	)
}
