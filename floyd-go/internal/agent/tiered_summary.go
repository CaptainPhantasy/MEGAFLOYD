package agent

import (
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

const preservedSummaryHeading = "\n\n## Preserved conversation requirements and decisions\n"

// tieredSummaryMessages sends a text transcript so compression cannot orphan a
// tool call/result pair in a provider's protocol. The source database is intact.
func tieredSummaryMessages(msgs []message.Message, tiered *TieredContext) []fantasy.Message {
	tiers := make(map[string]MessageTier)
	for _, msg := range tiered.Preserve {
		tiers[msg.ID] = Tier1Preserve
	}
	for _, msg := range tiered.Discard {
		tiers[msg.ID] = Tier3Discard
	}
	var transcript strings.Builder
	for _, msg := range msgs {
		tier := tiers[msg.ID]
		if tier == 0 {
			tier = Tier2Compress
		}
		fmt.Fprintf(&transcript, "\n[%s id=%s tier=%d]\n", msg.Role, msg.ID, tier)
		text := msg.Content().Text
		if tier == Tier3Discard {
			text = ""
		}
		transcript.WriteString(text)
		for _, call := range msg.ToolCalls() {
			fmt.Fprintf(&transcript, "\nTool %s arguments: %s\n", call.Name, call.Input)
		}
		for _, result := range msg.ToolResults() {
			content := result.Content
			if tier != Tier1Preserve {
				content = CompressToolResults(result.Name, content, result.IsError)
			}
			fmt.Fprintf(&transcript, "\nTool %s result (error=%t): %s\n", result.Name, result.IsError, content)
		}
	}
	return []fantasy.Message{fantasy.NewUserMessage(transcript.String())}
}

func formatSummaryTodos(todos []session.Todo) string {
	var out strings.Builder
	for _, todo := range todos {
		fmt.Fprintf(&out, "- [%s] %s\n", todo.Status, todo.Content)
	}
	return out.String()
}

func preservedSummaryContent(tiered *TieredContext) string {
	var out strings.Builder
	out.WriteString(tiered.preservedHistory)
	for _, msg := range tiered.Preserve {
		if text := msg.Content().Text; text != "" {
			fmt.Fprintf(&out, "\n[%s]\n%s\n", msg.Role, text)
		}
	}
	return out.String()
}
