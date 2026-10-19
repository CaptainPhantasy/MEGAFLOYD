package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/home"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

const TranscriptExportToolName = "transcript_export"

// TranscriptExportParams defines parameters for the transcript export tool
type TranscriptExportParams struct {
	// Reason for export (optional, used in export metadata)
	Reason string `json:"reason,omitempty"`
}

// TranscriptExportResponse contains the result of an export operation
type TranscriptExportResponse struct {
	FilePath     string `json:"file_path"`
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
	ToolCount    int    `json:"tool_count"`
	ExportedAt   string `json:"exported_at"`
}

// ToolExecution represents a single tool execution for the index
type ToolExecution struct {
	Timestamp string `json:"timestamp"`
	ToolName  string `json:"tool_name"`
	KeyParams string `json:"key_params"`
	IsError   bool   `json:"is_error"`
}

// NewTranscriptExportTool creates a tool that exports session transcripts
func NewTranscriptExportTool(sessions session.Service, messages message.Service, contextWindow int64, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TranscriptExportToolName,
		"Exports the current session transcript to a markdown file. Automatically triggered at 85% context usage or can be manually invoked.",
		func(ctx context.Context, params TranscriptExportParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("no active session"), nil
			}

			// Get session data
			sess, err := sessions.Get(ctx, sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to get session: %v", err)), nil
			}

			// Get all messages for the session
			msgs, err := messages.List(ctx, sessionID)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to get messages: %v", err)), nil
			}

			// Calculate context percentage
			totalTokens := sess.PromptTokens + sess.CompletionTokens
			percentUsed := 0.0
			if contextWindow > 0 {
				percentUsed = (float64(totalTokens) / float64(contextWindow)) * 100
			}

			// Create transcripts directory
			transcriptsDir := filepath.Join(workingDir, ".floyd", "transcripts")
			if err := os.MkdirAll(transcriptsDir, 0755); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to create transcripts directory: %v", err)), nil
			}

			// Generate filename with timestamp
			timestamp := time.Now().UTC().Format("20060102_150405")
			filename := fmt.Sprintf("%s_%s.md", sessionID, timestamp)
			filePath := filepath.Join(transcriptsDir, filename)

			// Extract tool executions for indexing
			toolExecs := extractToolExecutions(msgs)

			// Generate transcript content
			content := generateTranscriptMarkdown(sess, msgs, toolExecs, percentUsed, totalTokens, contextWindow, workingDir, params.Reason)

			// Write the file
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to write transcript: %v", err)), nil
			}

			response := TranscriptExportResponse{
				FilePath:     filePath,
				SessionID:    sessionID,
				MessageCount: len(msgs),
				ToolCount:    len(toolExecs),
				ExportedAt:   time.Now().UTC().Format(time.RFC3339),
			}

			summary := fmt.Sprintf(
				"Transcript exported to: %s\n"+
					"Session: %s\n"+
					"Messages: %d | Tool executions: %d\n"+
					"Context: %.1f%% used",
				home.Short(filePath),
				sessionID,
				len(msgs),
				len(toolExecs),
				percentUsed,
			)

			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(summary), response), nil
		})
}

// extractToolExecutions pulls tool calls and results from messages for indexing
func extractToolExecutions(msgs []message.Message) []ToolExecution {
	var execs []ToolExecution

	for _, msg := range msgs {
		// Extract from tool result messages (role = "tool")
		if msg.Role == message.Tool {
			results := msg.ToolResults()
			for _, result := range results {
				keyParams := truncateParams(result.Content, 100)
				execs = append(execs, ToolExecution{
					Timestamp: formatTimestamp(msg.CreatedAt),
					ToolName:  result.Name,
					KeyParams: keyParams,
					IsError:   result.IsError,
				})
			}
		}

		// Also check assistant messages for tool calls
		if msg.Role == message.Assistant {
			calls := msg.ToolCalls()
			for _, call := range calls {
				keyParams := truncateParams(call.Input, 100)
				execs = append(execs, ToolExecution{
					Timestamp: formatTimestamp(msg.CreatedAt),
					ToolName:  call.Name,
					KeyParams: keyParams,
					IsError:   false,
				})
			}
		}
	}

	return execs
}

// truncateParams shortens parameter strings for the index table
func truncateParams(s string, maxLen int) string {
	// Clean up newlines and excess whitespace
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatTimestamp converts unix timestamp to readable format
func formatTimestamp(unix int64) string {
	if unix == 0 {
		return "N/A"
	}
	return time.Unix(unix, 0).UTC().Format("15:04:05")
}

// generateTranscriptMarkdown creates the full transcript markdown content
func generateTranscriptMarkdown(
	sess session.Session,
	msgs []message.Message,
	toolExecs []ToolExecution,
	percentUsed float64,
	totalTokens, contextWindow int64,
	workingDir, reason string,
) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Session Transcript: %s\n", sess.Title))
	sb.WriteString(fmt.Sprintf("> Session ID: %s\n", sess.ID))
	sb.WriteString(fmt.Sprintf("> Exported: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("> Project: %s\n", home.Short(workingDir)))
	sb.WriteString(fmt.Sprintf("> Model: (from session metadata)\n"))
	sb.WriteString(fmt.Sprintf("> Final Context: %.1f%% (%d / %d tokens)\n\n", percentUsed, totalTokens, contextWindow))

	if reason != "" {
		sb.WriteString(fmt.Sprintf("> Export Reason: %s\n\n", reason))
	}

	sb.WriteString("---\n\n")

	// Conversation Summary section
	sb.WriteString("## Conversation Summary\n\n")
	sb.WriteString(generateSummary(msgs))
	sb.WriteString("\n\n---\n\n")

	// Message Log section
	sb.WriteString("## Message Log\n\n")
	for _, msg := range msgs {
		sb.WriteString(formatMessageSection(msg))
	}
	sb.WriteString("\n---\n\n")

	// Tool Executions Index
	sb.WriteString("## Tool Executions Index\n\n")
	sb.WriteString(formatToolIndex(toolExecs))
	sb.WriteString("\n\n---\n\n")

	// Files Modified section (placeholder for now)
	sb.WriteString("## Files Modified\n\n")
	sb.WriteString("*File tracking to be implemented*\n\n")
	sb.WriteString("---\n\n")

	// Key Decisions section (placeholder for now)
	sb.WriteString("## Key Decisions\n\n")
	sb.WriteString("*Decision tracking to be implemented*\n\n")

	return sb.String()
}

// generateSummary creates a brief summary from the first user message
func generateSummary(msgs []message.Message) string {
	for _, msg := range msgs {
		if msg.Role == message.User {
			text := msg.Content().Text
			if len(text) > 500 {
				return text[:500] + "..."
			}
			if text == "" {
				return "*First user message contained non-text content*"
			}
			return text
		}
	}
	return "*No user messages in session*"
}

// formatMessageSection formats a single message for the transcript
func formatMessageSection(msg message.Message) string {
	var sb strings.Builder

	// Header based on role
	switch msg.Role {
	case message.User:
		sb.WriteString("### User\n")
		text := msg.Content().Text
		if text != "" {
			sb.WriteString(text + "\n\n")
		}
		// Check for code attachments
		for _, bin := range msg.BinaryContent() {
			if strings.HasPrefix(bin.MIMEType, "text/") {
				sb.WriteString(fmt.Sprintf("*(Attached: %s)*\n\n", bin.Path))
			}
		}

	case message.Assistant:
		sb.WriteString("### Assistant\n")
		text := msg.Content().Text
		if text != "" {
			sb.WriteString(text + "\n\n")
		}
		// Tool calls
		calls := msg.ToolCalls()
		if len(calls) > 0 {
			for _, call := range calls {
				sb.WriteString(fmt.Sprintf("**Tool Call: %s**\n", call.Name))
				sb.WriteString("```\n")
				sb.WriteString(call.Input)
				sb.WriteString("\n```\n\n")
			}
		}

	case message.Tool:
		results := msg.ToolResults()
		for _, result := range results {
			sb.WriteString(fmt.Sprintf("### Tool Result: %s\n", result.Name))
			if result.IsError {
				sb.WriteString("**Error:**\n")
			}
			sb.WriteString("```\n")
			content := result.Content
			if len(content) > 2000 {
				content = content[:2000] + "\n... (truncated)"
			}
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")
		}
	}

	return sb.String()
}

// formatToolIndex creates the tool executions table
func formatToolIndex(execs []ToolExecution) string {
	if len(execs) == 0 {
		return "*No tool executions recorded*\n"
	}

	var sb strings.Builder
	sb.WriteString("| Timestamp | Tool | Key Parameters | Status |\n")
	sb.WriteString("|-----------|------|----------------|--------|\n")

	for _, exec := range execs {
		status := "OK"
		if exec.IsError {
			status = "ERROR"
		}
		// Escape pipe characters in params
		params := strings.ReplaceAll(exec.KeyParams, "|", "\\|")
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			exec.Timestamp, exec.ToolName, params, status))
	}

	return sb.String()
}
