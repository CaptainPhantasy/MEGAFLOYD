package message

import (
	"context"
	"fmt"

	"github.com/legacy-ai/floyd/internal/db"
)

// ArchiveResult is a single hit from a technical archive search.
type ArchiveResult struct {
	Message Message `json:"message"`
}

// SearchTechnicalArchive queries the message database with a semantic
// filter: results are assistant tool-call turns, tool results, or user
// messages containing code blocks — plain prose is excluded so the
// archive surfaces technical context only.
func (s *service) SearchTechnicalArchive(ctx context.Context, projectPath, query string, limit int) ([]ArchiveResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("archive search unavailable: message service has no database connection")
	}
	if limit == 0 {
		limit = 5
	}

	queryPattern := "%" + query + "%"
	codeBlockPattern := "%" + "```" + "%"
	toolCallPattern := `%"type":"tool_call"%`

	sqlQuery := `
		SELECT id, session_id, role, parts, created_at
		FROM messages
		WHERE parts LIKE ?
		AND (
			(role = 'assistant' AND parts LIKE ?)
			OR role = 'tool'
			OR (role = 'user' AND parts LIKE ?)
		)
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, sqlQuery, queryPattern, toolCallPattern, codeBlockPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("archive search query failed: %w", err)
	}
	defer rows.Close()

	var results []ArchiveResult
	for rows.Next() {
		var dbMsg db.Message

		// Safely scan only the selected columns into the db.Message
		// struct; the remaining columns stay zero-valued and are not
		// used below.
		if err := rows.Scan(&dbMsg.ID, &dbMsg.SessionID, &dbMsg.Role, &dbMsg.Parts, &dbMsg.CreatedAt); err != nil {
			continue
		}

		msg, err := s.fromDBItem(dbMsg)
		if err != nil {
			// Skip malformed messages.
			continue
		}

		results = append(results, ArchiveResult{Message: msg})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}
