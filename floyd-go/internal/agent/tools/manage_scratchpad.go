package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/fantasy"
)

//go:embed manage_scratchpad.md
var manageScratchpadDescription []byte

type ManageScratchpadParams struct {
	Action  string `json:"action" jsonschema:"description=Action to perform (read, write, append, clear),enum=read,enum=write,enum=append,enum=clear"`
	Content string `json:"content,omitempty" jsonschema:"description=Content to write or append"`
}

func NewManageScratchpadTool(wd string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"manage_scratchpad",
		string(manageScratchpadDescription),
		func(ctx context.Context, params ManageScratchpadParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			path := filepath.Join(wd, ".floyd", "scratchpad.md")

			switch params.Action {
			case "read":
				b, err := os.ReadFile(path)
				if err != nil {
					if os.IsNotExist(err) {
						return fantasy.NewTextResponse("Scratchpad is empty."), nil
					}
					return fantasy.NewTextResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse(string(b)), nil
			case "write":
				os.MkdirAll(filepath.Dir(path), 0755)
				if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse("Scratchpad written."), nil
			case "append":
				os.MkdirAll(filepath.Dir(path), 0755)
				f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				defer f.Close()
				f.WriteString("\n" + params.Content)
				return fantasy.NewTextResponse("Scratchpad appended."), nil
			case "clear":
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse("Scratchpad cleared."), nil
			default:
				return fantasy.NewTextResponse(fmt.Sprintf("unknown action: %s", params.Action)), nil
			}
		},
	)
}
