package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/session"
)

const ContextStatusToolName = "context_status"

type ContextStatusParams struct{}

type ContextStatusResponse struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	TotalTokens      int64   `json:"total_tokens"`     // prompt + completion
	EffectiveTokens  int64   `json:"effective_tokens"` // prompt + completion - cached
	CachePercent     float64 `json:"cache_percent"`    // what % was served from cache
	ContextWindow    int64   `json:"context_window"`
	PercentUsed      float64 `json:"percent_used"` // based on TOTAL (conservative)
	RemainingTokens  int64   `json:"remaining_tokens"`
	ShouldHandoff    bool    `json:"should_handoff"` // approaching limit, prepare for handoff
	SessionID        string  `json:"session_id"`
}

func NewContextStatusTool(sessions session.Service, contextWindow int64) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ContextStatusToolName,
		"Returns current context window usage statistics. Use this to monitor your context consumption and know when to be more concise or when you have room for detailed responses.",
		func(ctx context.Context, params ContextStatusParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("no active session"), nil
			}

			sess, err := sessions.Get(ctx, sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to get session: %v", err)), nil
			}

			// TODO(port-wave3): the chassis session model does not track
			// cache-read tokens yet; report zero until it does.
			cacheReadTokens := int64(0)

			// Calculate both total and effective tokens
			totalTokens := sess.PromptTokens + sess.CompletionTokens
			effectiveTokens := totalTokens - cacheReadTokens

			// Calculate cache percentage
			cachePercent := 0.0
			if totalTokens > 0 {
				cachePercent = (float64(cacheReadTokens) / float64(totalTokens)) * 100
			}

			// Use TOTAL for percentage (conservative - shows actual context pressure)
			percentUsed := 0.0
			if contextWindow > 0 {
				percentUsed = (float64(totalTokens) / float64(contextWindow)) * 100
			}

			remaining := contextWindow - totalTokens

			// Determine warning threshold based on context window size
			// Large windows (>200k): warn when remaining < 20k tokens (~90% used)
			// Small windows: warn when remaining < 20% of window (~80% used)
			var warningThreshold int64
			if contextWindow > 200000 {
				warningThreshold = 20000
			} else {
				warningThreshold = int64(float64(contextWindow) * 0.2)
			}
			shouldWarn := remaining <= warningThreshold

			status := ContextStatusResponse{
				PromptTokens:     sess.PromptTokens,
				CompletionTokens: sess.CompletionTokens,
				CacheReadTokens:  cacheReadTokens,
				TotalTokens:      totalTokens,
				EffectiveTokens:  effectiveTokens,
				CachePercent:     cachePercent,
				ContextWindow:    contextWindow,
				PercentUsed:      percentUsed,
				RemainingTokens:  remaining,
				ShouldHandoff:    shouldWarn,
				SessionID:        sessionID,
			}

			// Human-readable summary with dual display
			var summary string
			if shouldWarn {
				summary = fmt.Sprintf(
					"⚠️ CONTEXT WARNING: %.1f%% of window used\n"+
						"  Total: %d tokens | Effective: %d tokens (%.0f%% cached)\n"+
						"  Remaining: %d tokens | Window: %d\n"+
						"  Session will compact when remaining tokens fall below threshold.",
					percentUsed,
					totalTokens, effectiveTokens, cachePercent,
					remaining, contextWindow,
				)
			} else {
				summary = fmt.Sprintf(
					"Context: %.1f%% used\n"+
						"  Total: %d tokens | Effective: %d tokens (%.0f%% cached)\n"+
						"  Remaining: %d tokens | Window: %d",
					percentUsed,
					totalTokens, effectiveTokens, cachePercent,
					remaining, contextWindow,
				)
			}

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(summary), status), nil
		})
}
