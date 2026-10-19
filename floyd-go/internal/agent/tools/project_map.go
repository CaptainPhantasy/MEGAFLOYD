package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
)

//go:embed project_map.md
var projectMapDescription []byte

type ProjectMapParams struct {
	MaxDepth int `json:"max_depth" jsonschema:"description=Maximum depth of the tree (default: 3)"`
}

func NewProjectMapTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"project_map",
		string(projectMapDescription),
		func(ctx context.Context, params ProjectMapParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			depth := 3
			if params.MaxDepth > 0 {
				depth = params.MaxDepth
			}

			var sb strings.Builder
			walkDir(wd, wd, "", depth, 0, &sb)

			return fantasy.NewTextResponse(sb.String()), nil
		},
	)
}

func walkDir(root string, path string, prefix string, maxDepth int, currentDepth int, sb *strings.Builder) {
	if currentDepth > maxDepth {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	for i, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".floyd" {
			continue
		}
		if name == "node_modules" || name == "vendor" {
			continue
		}

		isLast := i == len(entries)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, name))

		if entry.IsDir() {
			newPrefix := prefix + "│   "
			if isLast {
				newPrefix = prefix + "    "
			}
			walkDir(root, filepath.Join(path, name), newPrefix, maxDepth, currentDepth+1, sb)
		}
	}
}
