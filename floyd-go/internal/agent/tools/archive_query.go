package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/message"
)

const ArchiveQueryToolName = "query_floyd_archive"

type ArchiveQueryParams struct {
	Query string `json:"query" jsonschema:"description=The specific technical detail, tool execution, or code snippet to search for in past sessions."`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Max results to return. Default 5."`
}

func NewArchiveQueryTool(messages message.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ArchiveQueryToolName,
		"Query the persistent database for past code, tool results, and technical decisions. Use this whenever you lack historical context from previous sessions.",
		func(ctx context.Context, params ArchiveQueryParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.Query == "" {
				return fantasy.NewTextErrorResponse("Query cannot be empty"), nil
			}

			limit := params.Limit
			if limit == 0 {
				limit = 5
			}

			results, err := messages.SearchTechnicalArchive(ctx, workingDir, params.Query, limit)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Archive query failed: %v", err)), nil
			}

			if len(results) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("ARCHIVE RESULT: No technical records found matching '%s'. Do not guess; acknowledge the lack of data.", params.Query)), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("ARCHIVE RESULTS FOR: '%s'\n\n", params.Query))

			for _, r := range results {
				msg := r.Message
				sb.WriteString(fmt.Sprintf("--- [Session: %s | Role: %s | Created: %d] ---\n", msg.SessionID, msg.Role, msg.CreatedAt))

				// Extract tool calls from parts
				toolCalls := msg.ToolCalls()
				if len(toolCalls) > 0 {
					for _, tc := range toolCalls {
						sb.WriteString(fmt.Sprintf("EXECUTED TOOL: %s\n", tc.Name))
						if tc.Input != "" {
							input := tc.Input
							if len(input) > 500 {
								input = input[:500] + "\n... (truncated)"
							}
							sb.WriteString(fmt.Sprintf("INPUT: %s\n", input))
						}
					}
				}

				// Extract tool results from parts
				toolResults := msg.ToolResults()
				if len(toolResults) > 0 {
					for _, tr := range toolResults {
						sb.WriteString(fmt.Sprintf("TOOL RESULT [%s]:\n", tr.Name))
						content := tr.Content
						if len(content) > 1000 {
							content = content[:1000] + "\n... (truncated)"
						}
						sb.WriteString(fmt.Sprintf("%s\n", content))
					}
				}

				// Include text content if present
				textContent := msg.Content()
				if textContent.Text != "" {
					text := textContent.Text
					if len(text) > 1000 {
						text = text[:1000] + "\n... (truncated)"
					}
					sb.WriteString(fmt.Sprintf("CONTENT:\n%s\n", text))
				}
				sb.WriteString("\n")
			}

			return fantasy.NewTextResponse(sb.String()), nil
		})
}
