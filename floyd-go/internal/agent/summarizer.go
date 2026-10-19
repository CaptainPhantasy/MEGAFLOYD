package agent

import (
	"fmt"
	"strings"
)

// SummarizationLog tracks what was summarized for potential recall
type SummarizationLog struct {
	MessagesSummarized  int
	ToolCallsCompressed int
	FilesReferenced     []string
	KeyDecisions        []string
	DiscardedCount      int
}

// BuildTieredSummaryPrompt creates a tier-aware summary prompt
func BuildTieredSummaryPrompt(todos string, tc *TieredContext) string {
	var sb strings.Builder

	sb.WriteString("You are summarizing a conversation with SEMANTIC TIERED COMPRESSION.\n\n")

	sb.WriteString("## COMPRESSION RULES\n\n")

	// TIER 1 - PRESERVE
	sb.WriteString("### TIER 1 - PRESERVE (Do NOT summarize these):\n")
	for _, msg := range tc.Preserve {
		text := msg.Content().Text
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		sb.WriteString("- ")
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	// TIER 2 - COMPRESS guidelines
	sb.WriteString("\n### TIER 2 - COMPRESS (Extract key info):\n")
	sb.WriteString("- Tool calls: Preserve the tool name + purpose, compress result\n")
	sb.WriteString("- Exploration: Summarize as 'Searched X, found Y'\n")
	sb.WriteString("- Decisions: Capture what was decided and why\n")
	sb.WriteString("- Code locations: Preserve file:line references exactly\n")

	// TIER 3 - DISCARD
	sb.WriteString("\n### TIER 3 - DISCARD (Omit from summary):\n")
	sb.WriteString(fmt.Sprintf("- %d messages with verbose/duplicate content omitted\n", len(tc.Discard)))

	// Add key decisions section (NEW - semantic importance)
	if len(tc.KeyDecisions) > 0 {
		sb.WriteString("\n## KEY DECISIONS (SEMANTIC HIGH-VALUE)\n\n")
		seenDecisions := make(map[string]bool)
		for _, decision := range tc.KeyDecisions {
			if !seenDecisions[decision] {
				sb.WriteString("- ")
				sb.WriteString(decision)
				sb.WriteString("\n")
				seenDecisions[decision] = true
			}
		}
	}

	// Add file operations summary
	if len(tc.FileOperations) > 0 {
		sb.WriteString("\n## FILES ACCESSED\n\n")
		seenPaths := make(map[string]bool)
		for _, op := range tc.FileOperations {
			if op.Path != "" && !seenPaths[op.Path] {
				sb.WriteString("- ")
				sb.WriteString(op.Action)
				sb.WriteString(": ")
				sb.WriteString(op.Path)
				sb.WriteString("\n")
				seenPaths[op.Path] = true
			}
		}
	}

	// Add tool summary
	if len(tc.ToolSummaries) > 0 {
		sb.WriteString("\n## TOOLS USED\n\n")
		toolCounts := make(map[string]int)
		for _, ts := range tc.ToolSummaries {
			toolCounts[ts.Name]++
		}
		for name, count := range toolCounts {
			sb.WriteString(fmt.Sprintf("- %s (%d times)\n", name, count))
		}
	}

	// Add todo list
	if todos != "" {
		sb.WriteString("\n## CURRENT TODO LIST\n\n")
		sb.WriteString(todos)
		sb.WriteString("\nInclude these tasks in your summary. ")
		sb.WriteString("Instruct the resuming assistant to use the `todos` tool.\n")
	}

	// Summary format instructions
	sb.WriteString("\n## SUMMARY SECTIONS\n\n")
	sb.WriteString("1. **Original Request** - The exact user request (from TIER 1)\n")
	sb.WriteString("2. **Current State** - Progress, what's done, what's in progress\n")
	sb.WriteString("3. **Files & Changes** - Files modified/read with line numbers (PRESERVE EXACT REFERENCES)\n")
	sb.WriteString("4. **Technical Context** - Decisions made, patterns identified, commands used\n")
	sb.WriteString("5. **Next Steps** - Specific, actionable next steps with code locations if known\n\n")

	sb.WriteString("**Tone**: Brief a teammate taking over. No emojis. Be thorough but concise.\n")
	sb.WriteString("**CRITICAL**: Preserve any file:line references (e.g., `foo.go:123`) exactly.\n")

	return sb.String()
}

// CompressToolResults creates a brief summary of tool results
func CompressToolResults(toolName, content string, isError bool) string {
	if isError {
		return fmt.Sprintf("[ERROR] %s: failed", toolName)
	}

	// Truncate long results
	if len(content) > 200 {
		// Try to find a good truncation point
		lines := strings.Split(content, "\n")
		if len(lines) > 0 && len(lines[0]) <= 200 {
			return lines[0] + "..."
		}
		if len(content) > 200 {
			return content[:197] + "..."
		}
	}

	return content
}

// CreateSummarizationLog builds a log from tiered context
func CreateSummarizationLog(tc *TieredContext) *SummarizationLog {
	log := &SummarizationLog{
		MessagesSummarized: len(tc.Compress),
		DiscardedCount:     len(tc.Discard),
		FilesReferenced:    make([]string, 0),
		KeyDecisions:       tc.KeyDecisions,
	}

	// Count tool calls
	log.ToolCallsCompressed = len(tc.ToolSummaries)

	// Collect unique file paths
	seenPaths := make(map[string]bool)
	for _, op := range tc.FileOperations {
		if op.Path != "" && !seenPaths[op.Path] {
			log.FilesReferenced = append(log.FilesReferenced, op.Path)
			seenPaths[op.Path] = true
		}
	}

	return log
}

// FormatSummaryStats returns a human-readable summary of compression stats
func FormatSummaryStats(tc *TieredContext) string {
	preserve, compress, discard := tc.GetStats()
	return fmt.Sprintf("Preserved: %d, Compressed: %d, Discarded: %d", preserve, compress, discard)
}
