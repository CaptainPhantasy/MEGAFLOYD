package hookruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/filelock"
	"github.com/legacy-ai/floyd/internal/filepathext"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/tidwall/sjson"
)

// ExecuteRegisteredBody checks the actual registered schema after hook rewrites
// and coordinates known native file writes. It does not grant permissions.
func (r *Runtime) ExecuteRegisteredBody(ctx context.Context, info fantasy.ToolInfo, call fantasy.ToolCall, run func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error)) (fantasy.ToolResponse, error) {
	err := validateInput(info, call.Input)
	outcome, state := "validated", "succeeded"
	if err != nil {
		outcome, state = "rejected", "denied"
	}
	r.recordBuiltin(ctx, call, hooks.EventPreToolUse, "registered-tool-schema", state, outcome, nil)
	if err != nil {
		RejectTool(ctx)
		return fantasy.NewTextErrorResponse("Tool input does not match its registered schema: " + err.Error()), nil
	}
	cwd := ""
	if r != nil && r.config != nil {
		cwd = r.config.WorkingDir()
	}
	paths, err := nativeWritePaths(call, cwd)
	if err != nil {
		RejectTool(ctx)
		return fantasy.NewTextErrorResponse("Cannot resolve write target: " + err.Error()), nil
	}
	release, err := filelock.Acquire(ctx, paths)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	defer release()
	if state, _ := ctx.Value(actionKey{}).(*actionState); state != nil {
		state.mu.Lock()
		state.mechanical = true
		state.mu.Unlock()
	}
	EnterTool(ctx)
	response, err := run(ctx, call)
	metadata := response.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	if updated, e := sjson.Set(metadata, "mechanical", map[string]any{"version": 1, "registered_schema": true, "native_write_lock_count": len(paths)}); e == nil {
		response.Metadata = updated
	}
	return response, err
}

func validateInput(info fantasy.ToolInfo, input string) error {
	data, err := json.Marshal(map[string]any{"type": "object", "properties": info.Parameters, "required": info.Required})
	if err != nil {
		return err
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(data, &schema); err != nil {
		return err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("invalid registered schema: %w", err)
	}
	var value any
	if err = json.Unmarshal([]byte(input), &value); err != nil {
		return err
	}
	return resolved.Validate(value)
}

func nativeWritePaths(call fantasy.ToolCall, cwd string) ([]string, error) {
	var input struct {
		FilePath string `json:"file_path"`
		Patch    string `json:"patch"`
	}
	var names []string
	switch call.Name {
	case "write", "edit", "multiedit":
		if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
			return nil, err
		}
		// Match these tools' existing SmartJoin semantics without rewriting input.
		if input.FilePath != "" {
			names = append(names, filepathext.SmartJoin(cwd, input.FilePath))
		}
	case "apply_patch":
		if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
			return nil, err
		}
		for _, line := range strings.Split(input.Patch, "\n") {
			if !strings.HasPrefix(line, "+++ ") && !strings.HasPrefix(line, "--- ") {
				continue
			}
			name := strings.TrimSpace(strings.SplitN(line[4:], "\t", 2)[0])
			name = strings.Trim(name, "\"")
			if name == "" || name == "/dev/null" {
				continue
			}
			name = strings.TrimPrefix(strings.TrimPrefix(name, "a/"), "b/")
			names = append(names, name)
		}
	default:
		return nil, nil
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		path, err := filelock.Resolve(cwd, name)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func (r *Runtime) recordBuiltin(ctx context.Context, call fantasy.ToolCall, event, name, state, outcome string, exit *int) {
	if r == nil {
		return
	}
	info := details(ctx)
	sid := ""
	if action, _ := ctx.Value(actionKey{}).(*actionState); action != nil {
		sid = action.sessionID
	}
	now := time.Now().UnixNano()
	r.record(hooks.Execution{ID: uuid.NewString(), EventID: info.EventID, Event: event, SessionID: sid, ToolCallID: call.ID, RunID: info.RunID, ParentToolCallID: info.ParentToolCallID, ToolName: call.Name, HookName: name, ConfigIndex: -1, Mode: "builtin", Required: false, State: state, Outcome: outcome, StartedAt: now, FinishedAt: now, ExitCode: exit})
}
