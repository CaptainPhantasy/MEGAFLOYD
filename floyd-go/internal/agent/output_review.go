package agent

import (
	"context"
	_ "embed"
	"log/slog"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/message"
)

//go:embed templates/output_review.md
var outputReviewPrompt []byte

// Output guidance applies inside the original generation, never another call.
func (a *sessionAgent) outputPrompt(prompt string) string {
	if !message.OutputReviewEnabled(a.messages) {
		return prompt
	}
	return prompt + "\n\n" + string(outputReviewPrompt)
}

func (a *sessionAgent) publicOutputResult(ctx context.Context, result *fantasy.AgentResult, id string) *fantasy.AgentResult {
	out := *result
	out.Steps = nil
	text, err := message.FinalPublicText(ctx, a.messages, id)
	if err != nil {
		// The public projection is unavailable (review pending, withheld,
		// or a lookup failure). Callers of Run must not receive a silently
		// blank success, so keep the run's own content. Public surfaces are
		// unaffected: they read through FinalPublicText/PublicGet and stay
		// gated by the review state.
		slog.Warn("Public output projection unavailable; returning run content",
			"message_id", id, "error", err)
		return &out
	}
	if text != "" {
		out.Response.Content = append(out.Response.Content[:0], fantasy.TextContent{Text: text})
	}
	return &out
}
