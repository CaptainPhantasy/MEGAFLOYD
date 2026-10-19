package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"

	"charm.land/fantasy"
)

//go:embed list_symbols.md
var listSymbolsDescription []byte

type ListSymbolsParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the file to inspect"`
}

func NewListSymbolsTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_symbols",
		string(listSymbolsDescription),
		func(ctx context.Context, params ListSymbolsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			pattern := `^(func|type|interface|class|const|var|export class|export function|export const) `
			
			cmd := exec.CommandContext(ctx, "grep", "-En", pattern, params.FilePath)
			cmd.Dir = wd
			out, _ := cmd.CombinedOutput() 
			
			if len(out) == 0 {
				pattern = `(class .* \{|function .*\(|const .* = .*=>|let .* = .*=>)`
				cmd = exec.CommandContext(ctx, "grep", "-En", pattern, params.FilePath)
				cmd.Dir = wd
				out, _ = cmd.CombinedOutput()
			}

			res := string(out)
			if res == "" {
				res = "No standard structural symbols found using lightweight scan."
			}
			
			return fantasy.NewTextResponse(fmt.Sprintf("Structural symbols found:\n%s", res)), nil
		},
	)
}
