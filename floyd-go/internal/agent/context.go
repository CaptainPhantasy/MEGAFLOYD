package agent

import (
	"regexp"
	"strings"

	"github.com/legacy-ai/floyd/internal/message"
)

// MessageTier represents the importance level of a message for summarization
type MessageTier int

const (
	// Tier1Preserve - Never summarize, always keep full content
	// System prompt, first user request, explicit requirements
	Tier1Preserve MessageTier = 1

	// Tier2Compress - Intelligently compress while preserving key info
	// Tool calls/results, exploration, decisions
	Tier2Compress MessageTier = 2

	// Tier3Discard - Can be discarded during summarization
	// Duplicates, verbose output, failed branches
	Tier3Discard MessageTier = 3
)

// TieredContext holds messages organized by tier
type TieredContext struct {
	Preserve []message.Message
	Compress []message.Message
	Discard  []message.Message

	// Metadata for summarization
	ToolSummaries    []ToolSummary
	FileOperations   []FileOperation
	KeyDecisions     []string
	preservedHistory string
}

// ToolSummary captures essential info from tool interactions
type ToolSummary struct {
	Name    string
	Action  string
	Outcome string
}

// FileOperation tracks file read/write operations
type FileOperation struct {
	Path    string
	Action  string
	Summary string
}

// preserveKeywords indicate user messages with explicit requirements
var preserveKeywords = []string{
	"requirement", "must", "need to", "important", "critical",
	"don't", "do not", "never", "always", "essential",
	"constraint", "requirement", "spec", "specification",
}

// semanticHighImportance patterns indicate content that should be preserved
// These capture: decisions, code locations, successful fixes, errors resolved
var semanticHighImportancePatterns = []*regexp.Regexp{
	// Code locations (file:line references)
	regexp.MustCompile(`(?i)(\S+\.go|\S+\.ts|\S+\.js|\S+\.py|\S+\.rs):\d+`),
	regexp.MustCompile(`(?i)function\s+\w+`),
	regexp.MustCompile(`(?i)class\s+\w+`),
	regexp.MustCompile(`(?i)in\s+\S+\.go`),
	regexp.MustCompile(`(?i)at\s+\S+:\d+`),
	// Decisions and rationale
	regexp.MustCompile(`(?i)(decided|chose|selected|determined|concluded)\s+(to|because|that|on)`),
	regexp.MustCompile(`(?i)because\s+\w+`),
	regexp.MustCompile(`(?i)reason:\s*`),
	regexp.MustCompile(`(?i)rationale:\s*`),
	// Successful outcomes
	regexp.MustCompile(`(?i)(fixed|resolved|implemented|added|created|updated|modified)\s+(successfully|the\s+\w+|in\s+\S+)`),
	regexp.MustCompile(`(?i)error\s+(fixed|resolved|handled)`),
	regexp.MustCompile(`(?i)working\s+now`),
	regexp.MustCompile(`(?i)succeeded`),
	// Key state changes
	regexp.MustCompile(`(?i)(error|bug|issue|problem)\s*(was|is|has\s+been)\s*(in|at|in\s+\S+)`),
	regexp.MustCompile(`(?i)root\s+cause`),
	regexp.MustCompile(`(?i)the\s+fix\s+(is|was)`),
}

// semanticLowImportance patterns indicate content that can be compressed
var semanticLowImportancePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(okay|ok|sure|yes|no|done|got it|alright)$`),
	regexp.MustCompile(`(?i)^(let me|I'll|I will)\s+(check|look|try|see)`),
	regexp.MustCompile(`(?i)^(searching|looking|checking|trying)\s*\.\.\.$`),
}

// SemanticScore represents the importance of a message for compaction
type SemanticScore struct {
	HasCodeLocation bool
	HasDecision     bool
	HasFixOutcome   bool
	IsLowImportance bool
	RawScore        int
}

// CalculateSemanticScore analyzes message content for semantic importance
func CalculateSemanticScore(msg message.Message, position, totalMessages int) SemanticScore {
	score := SemanticScore{}
	text := msg.Content().Text

	// Check for high importance patterns
	for _, pattern := range semanticHighImportancePatterns {
		patternStr := pattern.String()
		if pattern.MatchString(text) {
			score.RawScore += 3
			if strings.Contains(patternStr, `\.`) || strings.Contains(patternStr, `:\d+`) {
				score.HasCodeLocation = true
			}
			if strings.Contains(patternStr, "decid") || strings.Contains(patternStr, "chose") || strings.Contains(patternStr, "because") || strings.Contains(patternStr, "rationale") {
				score.HasDecision = true
			}
			if strings.Contains(patternStr, "fix") || strings.Contains(patternStr, "resolv") || strings.Contains(patternStr, "success") || strings.Contains(patternStr, "implement") {
				score.HasFixOutcome = true
			}
		}
	}

	// Check for low importance patterns
	for _, pattern := range semanticLowImportancePatterns {
		if pattern.MatchString(text) {
			score.IsLowImportance = true
			score.RawScore -= 2
		}
	}

	// Recency weighting: recent messages (last 30%) get +1, very recent (last 10%) get +2
	if totalMessages > 0 {
		positionRatio := float64(position) / float64(totalMessages)
		if positionRatio > 0.9 {
			score.RawScore += 2
		} else if positionRatio > 0.7 {
			score.RawScore += 1
		}
	}

	// Tool results with errors that were resolved get higher score
	if msg.Role == message.Tool {
		results := msg.ToolResults()
		for _, result := range results {
			if !result.IsError && len(result.Content) > 50 {
				// Successful tool result with substantial content
				score.RawScore += 1
			}
		}
	}

	return score
}

// ClassifyMessage determines the tier for a message
func ClassifyMessage(msg message.Message, isFirstUser bool) MessageTier {
	return ClassifyMessageWithScore(msg, isFirstUser, 0, 1, SemanticScore{})
}

// ClassifyMessageWithScore determines tier using semantic importance scoring
func ClassifyMessageWithScore(msg message.Message, isFirstUser bool, position, totalMessages int, score SemanticScore) MessageTier {
	if msg.Role == message.System {
		return Tier1Preserve
	}
	// TIER 1: First user message (original request) - ALWAYS preserve
	if isFirstUser && msg.Role == message.User {
		return Tier1Preserve
	}

	// TIER 1: User messages with explicit requirements
	if msg.Role == message.User {
		text := strings.ToLower(msg.Content().Text)
		for _, kw := range preserveKeywords {
			if strings.Contains(text, kw) {
				return Tier1Preserve
			}
		}
		// High semantic score user messages are also Tier 1
		if score.RawScore >= 3 {
			return Tier1Preserve
		}
	}

	// TIER 1: Messages with code locations, decisions, or fixes (high semantic value)
	if score.RawScore >= 4 {
		return Tier1Preserve
	}

	// TIER 3: Error tool results (keep resolution, discard details)
	if msg.Role == message.Tool {
		results := msg.ToolResults()
		for _, result := range results {
			if result.IsError {
				// BUT: if this error was followed by a fix, promote to Tier2
				if score.HasFixOutcome {
					return Tier2Compress
				}
				return Tier3Discard
			}
		}

		// TIER 3: Very long tool results (>3000 chars = verbose output)
		// BUT: preserve if it has high semantic score (code locations, decisions)
		for _, result := range results {
			if len(result.Content) > 3000 && score.RawScore < 2 {
				return Tier3Discard
			}
		}
	}

	// TIER 2: Low importance messages can be compressed more aggressively
	if score.IsLowImportance && score.RawScore < 1 {
		return Tier2Compress // Still compressible, just noted
	}

	// TIER 2: Everything else (tool calls, normal messages, etc.)
	return Tier2Compress
}

// PrepareTieredContext organizes messages by tier with semantic scoring
func PrepareTieredContext(msgs []message.Message) *TieredContext {
	tc := &TieredContext{
		Preserve: make([]message.Message, 0),
		Compress: make([]message.Message, 0),
		Discard:  make([]message.Message, 0),
	}

	totalMessages := len(msgs)
	foundUser := false

	for i, msg := range msgs {
		if msg.IsSummaryMessage {
			tc.Compress = append(tc.Compress, msg)
			if _, preserved, ok := strings.Cut(msg.Content().Text, preservedSummaryHeading); ok {
				tc.preservedHistory = preserved
			}
			continue
		}
		// Calculate semantic score for this message
		score := CalculateSemanticScore(msg, i, totalMessages)
		firstUser := msg.Role == message.User && !foundUser
		if msg.Role == message.User {
			foundUser = true
		}
		tier := ClassifyMessageWithScore(msg, firstUser, i, totalMessages, score)

		switch tier {
		case Tier1Preserve:
			tc.Preserve = append(tc.Preserve, msg)
			// Track decisions from high-value messages
			if score.HasDecision {
				extractedDecision := extractDecision(msg.Content().Text)
				if extractedDecision != "" {
					tc.KeyDecisions = append(tc.KeyDecisions, extractedDecision)
				}
			}
		case Tier2Compress:
			tc.Compress = append(tc.Compress, msg)
		case Tier3Discard:
			tc.Discard = append(tc.Discard, msg)
		}

		// Extract tool summaries from TIER 2 messages
		if tier == Tier2Compress {
			tc.extractToolInfo(msg)
		}
	}

	return tc
}

// extractDecision extracts a decision statement from message text
func extractDecision(text string) string {
	// Look for decision patterns and extract the core statement
	decisionPatterns := []string{
		"decided", "chose", "selected", "determined", "concluded",
	}

	lowerText := strings.ToLower(text)
	for _, pattern := range decisionPatterns {
		idx := strings.Index(lowerText, pattern)
		if idx >= 0 {
			// Extract from the pattern to the next sentence or 200 chars
			start := idx
			end := idx + 200
			if end > len(text) {
				end = len(text)
			}
			extracted := text[start:end]
			// Trim at sentence boundary
			if periodIdx := strings.Index(extracted, ". "); periodIdx > 20 {
				extracted = extracted[:periodIdx+1]
			}
			return strings.TrimSpace(extracted)
		}
	}
	return ""
}

// extractToolInfo extracts tool operation summaries from a message
func (tc *TieredContext) extractToolInfo(msg message.Message) {
	// Extract from assistant messages with tool calls
	if msg.Role == message.Assistant {
		calls := msg.ToolCalls()
		for _, call := range calls {
			tc.ToolSummaries = append(tc.ToolSummaries, ToolSummary{
				Name: call.Name,
			})
		}
	}

	// Extract file operations from tool results
	if msg.Role == message.Tool {
		results := msg.ToolResults()
		for _, result := range results {
			if isFileTool(result.Name) {
				tc.FileOperations = append(tc.FileOperations, FileOperation{
					Path:   extractFilePath(result.Content),
					Action: inferFileAction(result.Name),
				})
			}
		}
	}
}

// isFileTool checks if a tool name relates to file operations
func isFileTool(name string) bool {
	fileTools := []string{"view", "write", "edit", "multiedit", "glob", "grep", "ls"}
	nameLower := strings.ToLower(name)
	for _, t := range fileTools {
		if strings.Contains(nameLower, t) {
			return true
		}
	}
	return false
}

// extractFilePath attempts to extract a file path from tool output
func extractFilePath(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for common path patterns
		if strings.HasPrefix(line, "/") || strings.HasPrefix(line, "./") || strings.HasPrefix(line, "~/") {
			// Truncate to reasonable length
			if len(line) > 100 {
				return line[:100]
			}
			return line
		}
	}
	if len(lines) > 0 && len(lines[0]) < 100 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// inferFileAction infers the action type from tool name
func inferFileAction(name string) string {
	nameLower := strings.ToLower(name)
	switch {
	case strings.Contains(nameLower, "write"):
		return "created"
	case strings.Contains(nameLower, "edit"):
		return "modified"
	case strings.Contains(nameLower, "view"):
		return "read"
	case strings.Contains(nameLower, "glob") || strings.Contains(nameLower, "grep"):
		return "searched"
	case strings.Contains(nameLower, "ls"):
		return "listed"
	default:
		return "accessed"
	}
}

// GetStats returns statistics about the tiered context
func (tc *TieredContext) GetStats() (preserve, compress, discard int) {
	return len(tc.Preserve), len(tc.Compress), len(tc.Discard)
}
