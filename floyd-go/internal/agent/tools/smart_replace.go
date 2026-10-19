package tools

import (
	"context"
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
)

//go:embed smart_replace.md
var smartReplaceDescription []byte

type SmartReplaceParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the file"`
	Search   string `json:"search" jsonschema:"description=The exact or nearly-exact block to find"`
	Replace  string `json:"replace" jsonschema:"description=The new text block"`
}

func NewSmartReplaceTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"smart_replace",
		string(smartReplaceDescription),
		func(ctx context.Context, params SmartReplaceParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			fullPath := filepath.Join(wd, params.FilePath)
			b, err := os.ReadFile(fullPath)
			if err != nil {
				return fantasy.NewTextResponse(err.Error()), nil
			}
			content := string(b)

			if strings.Contains(content, params.Search) {
				newContent := strings.Replace(content, params.Search, params.Replace, 1)
				if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
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
