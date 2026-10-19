package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/filepathext"
	"github.com/legacy-ai/floyd/internal/fsext"
	"github.com/legacy-ai/floyd/internal/permission"
)

//go:embed smart_replace.md
var smartReplaceDescription []byte

type SmartReplaceParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the file"`
	Search   string `json:"search" jsonschema:"description=The exact or nearly-exact block to find"`
	Replace  string `json:"replace" jsonschema:"description=The new text block"`
}

func NewSmartReplaceTool(permissions permission.Service, wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"smart_replace",
		string(smartReplaceDescription),
		func(ctx context.Context, params SmartReplaceParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			fullPath := filepathext.SmartJoin(wd, params.FilePath)
			if pathErr := checkToolPath(permissions, fullPath); pathErr != nil {
				return deniedPathResponse(pathErr), nil
			}
			b, err := os.ReadFile(fullPath)
			if err != nil {
				return fantasy.NewTextResponse(err.Error()), nil
			}
			content := string(b)

			if strings.Contains(content, params.Search) {
				newContent := strings.Replace(content, params.Search, params.Replace, 1)

				granted, err := permissions.Request(
					ctx,
					permission.CreatePermissionRequest{
						SessionID:   GetSessionFromContext(ctx),
						Path:        fsext.PathOrPrefix(fullPath, wd),
						ToolCallID:  call.ID,
						ToolName:    "smart_replace",
						Action:      "write",
						Description: fmt.Sprintf("Replace text in %s", fullPath),
						Params:      params,
					},
				)
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				if !granted {
					return NewPermissionDeniedResponse(), nil
				}

				if err := os.WriteFile(fullPath, []byte(newContent), 0o644); err != nil {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse("Strict replace successful."), nil
			}

			// basic whitespace-invariant check
			normalizedContent := strings.Join(strings.Fields(content), " ")
			normalizedSearch := strings.Join(strings.Fields(params.Search), " ")
			if strings.Contains(normalizedContent, normalizedSearch) {
				return fantasy.NewTextResponse("Error: 'search' block not found exactly, but a whitespace-invariant match exists. Check your spacing/indentation and use the exact text, or use edit/apply_patch."), nil
			}

			return fantasy.NewTextResponse("Error: 'search' block not found. Try providing more context or verifying exact whitespace."), nil
		},
	)
}
