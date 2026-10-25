package tools

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/megafloyd"
)

//go:embed manage_scratchpad.md
var manageScratchpadDescription []byte

type ManageScratchpadParams struct {
	Action  string `json:"action" jsonschema:"description=Action to perform (read, write, append, clear),enum=read,enum=write,enum=append,enum=clear"`
	Content string `json:"content,omitempty" jsonschema:"description=Content to write or append"`
}

func NewManageScratchpadTool(wd string, selected ...string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"manage_scratchpad",
		string(manageScratchpadDescription),
		func(ctx context.Context, params ManageScratchpadParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var explicit string
			if len(selected) > 0 {
				explicit = selected[0]
			}
			dataDir, err := megafloyd.PersonalDataDir(wd, explicit)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			path := filepath.Join(dataDir, "scratchpad.md")

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
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					return fantasy.ToolResponse{}, err
				}
				if err := os.WriteFile(path, []byte(params.Content), 0o600); err != nil {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				return fantasy.NewTextResponse("Scratchpad written."), nil
			case "append":
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					return fantasy.ToolResponse{}, err
				}
				f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return fantasy.NewTextResponse(err.Error()), nil
				}
				defer f.Close()
				if _, err := f.WriteString("\n" + params.Content); err != nil {
					return fantasy.ToolResponse{}, err
				}
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
