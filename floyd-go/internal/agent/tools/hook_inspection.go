package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
)

const (
	HookExplainToolName = "hook_explain"
	HookTraceToolName   = "hook_trace"
)

//go:embed hook_explain.md
var hookExplainDescription string

//go:embed hook_trace.md
var hookTraceDescription string

func init() {
	RegisterBuiltin(HookExplainToolName, "Preview configured hook matching and capabilities without execution", "diagnostics")
	RegisterBuiltin(HookTraceToolName, "Read persisted hook event and job outcomes", "diagnostics")
}

type HookExplainParams struct {
	Event    string `json:"event" jsonschema:"description=Hook event such as PreToolUse or PostToolUseFailure"`
	ToolName string `json:"tool_name,omitempty" jsonschema:"description=Exact tool name to match; omit for events without a tool"`
}

type HookPlanRow struct {
	hooks.PlanEntry
	ConfigKey string `json:"config_key"`
}

type HookExplanation struct {
	Event          string        `json:"event"`
	ToolName       string        `json:"tool_name"`
	Source         string        `json:"source"`
	InspectionOnly bool          `json:"inspection_only"`
	Entries        []HookPlanRow `json:"entries"`
}

// ExplainHooks reads the effective hook configuration without executing it.
func ExplainHooks(cfg *config.Config, eventName, toolName, cwd, source string) (HookExplanation, error) {
	if cfg == nil {
		return HookExplanation{}, errors.New("hook configuration is unavailable")
	}
	eventName, err := config.NormalizeHookEvent(strings.TrimSpace(eventName))
	if err != nil {
		return HookExplanation{}, err
	}
	runner := hooks.NewRunner(cfg.Hooks[eventName], cwd, cwd)
	entries := runner.Preview(eventName, toolName)
	result := HookExplanation{
		Event: eventName, ToolName: toolName, Source: source,
		InspectionOnly: true, Entries: make([]HookPlanRow, 0, len(entries)),
	}
	for _, entry := range entries {
		result.Entries = append(result.Entries, HookPlanRow{
			PlanEntry: entry,
			ConfigKey: fmt.Sprintf("hooks.%s[%d]", eventName, entry.ConfigIndex),
		})
	}
	return result, nil
}

func NewHookExplainTool(cfg *config.ConfigStore) fantasy.AgentTool {
	return fantasy.NewAgentTool(HookExplainToolName, hookExplainDescription,
		func(ctx context.Context, params HookExplainParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := ctx.Err(); err != nil {
				return fantasy.ToolResponse{}, err
			}
			if cfg == nil {
				return fantasy.NewTextErrorResponse("hook configuration is unavailable"), nil
			}
			result, err := ExplainHooks(cfg.Config(), params.Event, params.ToolName, cfg.WorkingDir(), "effective_runtime")
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return hookInspectionResponse(result)
		})
}

type HookTraceParams struct {
	SessionID  string `json:"session_id,omitempty" jsonschema:"description=Exact session ID in this workspace; defaults to the current session"`
	ToolCallID string `json:"tool_call_id,omitempty" jsonschema:"description=Exact tool call ID; combined with other supplied filters"`
	EventID    string `json:"event_id,omitempty" jsonschema:"description=Exact hook event ID"`
	JobID      string `json:"job_id,omitempty" jsonschema:"description=Exact asynchronous hook job ID"`
	After      int64  `json:"after,omitempty" jsonschema:"description=Exclusive sequence cursor from an earlier page,minimum=0"`
	Limit      int    `json:"limit,omitempty" jsonschema:"description=Page size; defaults to 100 and accepts 1 through 500,minimum=0,maximum=500"`
}

// Query validates pagination before opening or reading a hook store.
func (p HookTraceParams) Query(defaultSessionID string) (hookstore.Query, error) {
	if p.After < 0 {
		return hookstore.Query{}, errors.New("after must be zero or a positive sequence")
	}
	if p.Limit < 0 || p.Limit > 500 {
		return hookstore.Query{}, errors.New("limit must be between 1 and 500, or zero for the default")
	}
	if p.Limit == 0 {
		p.Limit = 100
	}
	if p.SessionID == "" {
		p.SessionID = defaultSessionID
	}
	return hookstore.Query{
		SessionID: p.SessionID, ToolCallID: p.ToolCallID,
		EventID: p.EventID, JobID: p.JobID, After: p.After, Limit: p.Limit,
	}, nil
}

func NewHookTraceTool(reader hookstore.Reader) fantasy.AgentTool {
	if store, ok := reader.(*hookstore.Store); ok && store == nil {
		reader = nil
	}
	return fantasy.NewAgentTool(HookTraceToolName, hookTraceDescription,
		func(ctx context.Context, params HookTraceParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			query, err := params.Query(GetSessionFromContext(ctx))
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if reader == nil {
				return fantasy.NewTextErrorResponse("hook history is unavailable"), nil
			}
			page, err := reader.List(ctx, query)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("read hook history: %v", err)), nil
			}
			return hookInspectionResponse(page)
		})
}

func hookInspectionResponse(value any) (fantasy.ToolResponse, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fantasy.ToolResponse{}, fmt.Errorf("encode hook inspection: %w", err)
	}
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(string(data)), value), nil
}
