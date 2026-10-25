package agent

import (
	"context"
	_ "embed"

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
	out.Response.Content = nil
	text, err := message.FinalPublicText(ctx, a.messages, id)
	if err == nil && text != "" {
		out.Response.Content = append(out.Response.Content, fantasy.TextContent{Text: text})
	}
	return &out
}
