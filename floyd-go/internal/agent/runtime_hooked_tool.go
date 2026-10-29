package agent

import (
	"context"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/hookruntime"
)

// Runtime wrappers share the application lifecycle and recorder. Diagnostic
// tools remain read-only, even when a configured matcher selects every tool.
type runtimePreTool struct {
	fantasy.AgentTool
	runtime *hookruntime.Runtime
}
type runtimeObservedTool struct {
	fantasy.AgentTool
	runtime *hookruntime.Runtime
}

func isHookDiagnostic(tool fantasy.AgentTool) bool {
	name := tool.Info().Name
	return name == tools.HookExplainToolName || name == tools.HookTraceToolName
}

func wrapToolsWithRuntimePre(input []fantasy.AgentTool, runtime *hookruntime.Runtime) []fantasy.AgentTool {
	out := make([]fantasy.AgentTool, len(input))
	for i, tool := range input {
		out[i] = tool
		if !isHookDiagnostic(tool) {
			out[i] = &runtimePreTool{AgentTool: tool, runtime: runtime}
		}
	}
	return out
}

func wrapToolsWithRuntimeObservation(input []fantasy.AgentTool, runtime *hookruntime.Runtime) []fantasy.AgentTool {
	out := make([]fantasy.AgentTool, len(input))
	for i, tool := range input {
		out[i] = tool
		if !isHookDiagnostic(tool) {
			out[i] = &runtimeObservedTool{AgentTool: tool, runtime: runtime}
		}
	}
	return out
}

func (h *runtimePreTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	ctx, call, result, allowed := h.runtime.Before(ctx, tools.GetSessionFromContext(ctx), call)
	if !allowed {
		response := fantasy.NewTextErrorResponse("Tool call blocked by hook. Reason: " + result.Reason)
		response.StopTurn = result.Halt
		response.Metadata = hookruntime.MergeMetadata("", result)
		return response, nil
	}
	response, err := h.runtime.ExecuteRegisteredBody(ctx, h.AgentTool.Info(), call, h.AgentTool.Run)
	if result.Context != "" {
		response.Content = joinHookContext(response.Content, result.Context)
	}
	response.Metadata = hookruntime.MergeMetadata(response.Metadata, result)
	return response, err
}

func (h *runtimeObservedTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return h.runtime.ObserveTool(ctx, tools.GetSessionFromContext(ctx), call, h.AgentTool.Run)
}
