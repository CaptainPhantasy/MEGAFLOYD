package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/tidwall/sjson"
)

// hookedTool retains the legacy direct-runner adapter. Application tools use
// runtimePreTool and runtimeObservedTool with the shared lifecycle dispatcher.
type hookedTool struct {
	inner fantasy.AgentTool
	// pre runs PreToolUse hooks (built from the PreToolUse event group).
	// Nil skips the pre phase.
	pre *hooks.Runner
	// post runs PostToolUse hooks (built from the PostToolUse event
	// group). Nil skips the post phase. Runners are event-group-scoped:
	// one runner holds only the hooks configured for its event.
	post *hooks.Runner
}

func newHookedTool(inner fantasy.AgentTool, pre, post *hooks.Runner) *hookedTool {
	return &hookedTool{inner: inner, pre: pre, post: post}
}

// wrapToolsWithHooks preserves the legacy adapter contract for direct callers.
// The application does not use it: native child and parent tools both use the
// shared runtime wrappers, including terminal failure and cancellation events.
func wrapToolsWithHooks(tools []fantasy.AgentTool, pre, post *hooks.Runner, isSubAgent bool) []fantasy.AgentTool {
	if (pre == nil && post == nil) || isSubAgent {
		return tools
	}
	out := make([]fantasy.AgentTool, len(tools))
	for i, tool := range tools {
		out[i] = newHookedTool(tool, pre, post)
	}
	return out
}

func (h *hookedTool) Info() fantasy.ToolInfo {
	return h.inner.Info()
}

func (h *hookedTool) ProviderOptions() fantasy.ProviderOptions {
	return h.inner.ProviderOptions()
}

func (h *hookedTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	h.inner.SetProviderOptions(opts)
}

func (h *hookedTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	sessionID := tools.GetSessionFromContext(ctx)
	var result hooks.AggregateResult
	if h.pre != nil {
		pre, err := h.pre.Run(ctx, hooks.EventPreToolUse, sessionID, call.Name, call.Input)
		if err != nil {
			slog.Warn("Hook execution error, proceeding with tool call",
				"tool", call.Name, "error", err)
		}
		result = pre

		if result.Decision == hooks.DecisionDeny || result.Halt {
			reason := fmt.Sprintf("Tool call blocked by hook. Reason: %s", result.Reason)
			if result.Halt {
				reason = fmt.Sprintf("Turn halted by hook. Reason: %s", result.Reason)
			}
			resp := fantasy.NewTextErrorResponse(reason)
			// Halt ends the whole turn; a plain deny only blocks this tool
			// call so the model can see the error and try something else.
			resp.StopTurn = result.Halt
			resp.Metadata = hookMetadataJSON(result)
			return resp, nil
		}

		if result.UpdatedInput != "" {
			call.Input = result.UpdatedInput
		}

		// An explicit allow from a hook pre-approves the permission prompt
		// for this tool call. Deny is already handled above; silence falls
		// through to the normal permission flow.
		if result.Decision == hooks.DecisionAllow {
			ctx = permission.WithHookApproval(ctx, call.ID)
		}
	}

	resp, err := h.inner.Run(ctx, call)
	if err != nil {
		return resp, err
	}

	// PostToolUse hooks observe the completed tool call. They cannot
	// block (the tool already ran) or rewrite input; their decisions are
	// advisory, their context is appended to the response, and their
	// results merge into the hook metadata.
	if h.post != nil {
		postInput, postInputErr := sjson.Set(call.Input, "response", resp.Content)
		if postInputErr == nil {
			post, postErr := h.post.Run(ctx, hooks.EventPostToolUse, sessionID, call.Name, postInput)
			if postErr != nil {
				slog.Warn("PostToolUse hook execution error", "tool", call.Name, "error", postErr)
			}
			if post.Decision == hooks.DecisionDeny && post.Reason != "" {
				slog.Warn("PostToolUse hook denied after execution (advisory only)",
					"tool", call.Name, "reason", post.Reason)
			}
			result.HookCount += post.HookCount
			result.Hooks = append(result.Hooks, post.Hooks...)
			result.Context = joinHookContext(result.Context, post.Context)
		}
	}

	if result.Context != "" {
		if resp.Content != "" {
			resp.Content += "\n"
		}
		resp.Content += result.Context
	}

	resp.Metadata = mergeHookMetadata(resp.Metadata, result)
	return resp, nil
}

// joinHookContext concatenates two hook context fragments, keeping the
// separator consistent with hooks.aggregate.
func joinHookContext(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n" + b
	}
}

// buildHookMetadata creates a HookMetadata from an AggregateResult.
func buildHookMetadata(result hooks.AggregateResult) hooks.HookMetadata {
	return hooks.HookMetadata{
		HookCount:    result.HookCount,
		Decision:     result.Decision.String(),
		Halt:         result.Halt,
		Reason:       result.Reason,
		InputRewrite: result.UpdatedInput != "",
		Hooks:        result.Hooks,
	}
}

// hookMetadataJSON builds a JSON string containing only the hook metadata.
func hookMetadataJSON(result hooks.AggregateResult) string {
	meta := buildHookMetadata(result)
	data, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return `{"hook":` + string(data) + `}`
}

// mergeHookMetadata injects hook metadata into existing tool metadata.
func mergeHookMetadata(existing string, result hooks.AggregateResult) string {
	if result.HookCount == 0 {
		return existing
	}
	meta := buildHookMetadata(result)
	data, err := json.Marshal(meta)
	if err != nil {
		return existing
	}
	if existing == "" {
		existing = "{}"
	}
	merged, err := sjson.SetRaw(existing, "hook", string(data))
	if err != nil {
		return existing
	}
	return merged
}
