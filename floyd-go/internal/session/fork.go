package session

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// ForkResult contains the result of a session fork operation.
type ForkResult struct {
	NewSession   Session
	CopiedCount  int
	ForkMarkerID string
}

// Forker is implemented by session services that support forking a
// session's history into a new session. It is intentionally not part of
// the Service interface because several Service implementations (client
// workspaces, test doubles) cannot fork locally.
type Forker interface {
	Fork(ctx context.Context, sourceSessionID string, upToMessage int) (*ForkResult, error)
}

// Fork forks the given session through svc when the underlying service
// supports forking.
func Fork(ctx context.Context, svc Service, sourceSessionID string, upToMessage int) (*ForkResult, error) {
	forker, ok := svc.(Forker)
	if !ok {
		return nil, fmt.Errorf("session service does not support forking")
	}
	return forker.Fork(ctx, sourceSessionID, upToMessage)
}

// Fork creates a new session that is a copy of the source session up to
// (and including) the specified message index (0-based). The new session
// inherits the source session as its parent and receives a system message
// marking the fork point.
//
// upToMessage of -1 means copy all messages. upToMessage of 0 means
// copy only the system messages (no user messages).
func (s *service) Fork(ctx context.Context, sourceSessionID string, upToMessage int) (*ForkResult, error) {
	// 1. Validate the source session exists
	sourceDB, err := s.q.GetSessionByID(ctx, sourceSessionID)
	if err != nil {
		return nil, fmt.Errorf("source session %s: %w", sourceSessionID, err)
	}

	// 2. Get all messages from the source session
	sourceMsgs, err := s.q.ListMessagesBySession(ctx, sourceSessionID)
	if err != nil {
		return nil, fmt.Errorf("listing messages for session %s: %w", sourceSessionID, err)
	}

	// 3. Determine which messages to copy
	msgsToCopy := sourceMsgs
	if upToMessage >= 0 && upToMessage < len(sourceMsgs) {
		msgsToCopy = sourceMsgs[:upToMessage+1]
	} else if upToMessage >= 0 && upToMessage >= len(sourceMsgs) {
		// upToMessage exceeds available messages — copy all
		msgsToCopy = sourceMsgs
	}

	// 4. Create the forked session in a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin fork transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)

	newID := uuid.New().String()
	forkTitle := fmt.Sprintf("Fork of %s", sourceDB.Title)

	dbSession, err := qtx.CreateSession(ctx, db.CreateSessionParams{
		ID:              newID,
		ParentSessionID: sql.NullString{String: sourceSessionID, Valid: true},
		Title:           forkTitle,
	})
	if err != nil {
		return nil, fmt.Errorf("creating fork session: %w", err)
	}

	// 5. Copy messages
	for _, msg := range msgsToCopy {
		_, err := qtx.CreateMessage(ctx, db.CreateMessageParams{
			ID:               uuid.New().String(),
			SessionID:        newID,
			Role:             msg.Role,
			Parts:            msg.Parts,
			Model:            msg.Model,
			Provider:         msg.Provider,
			IsSummaryMessage: msg.IsSummaryMessage,
		})
		if err != nil {
			return nil, fmt.Errorf("copying message %s to fork: %w", msg.ID, err)
		}
	}

	// 6. Insert a fork marker system message
	markerContent := fmt.Sprintf(
		"[Fork] This session was forked from session %s at message index %d. Original title: %s",
		sourceSessionID, len(msgsToCopy)-1, sourceDB.Title,
	)
	markerParts := marshalForkMarker(markerContent)

	markerMsg, err := qtx.CreateMessage(ctx, db.CreateMessageParams{
		ID:               uuid.New().String(),
		SessionID:        newID,
		Role:             "system",
		Parts:            markerParts,
		IsSummaryMessage: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("inserting fork marker: %w", err)
	}

	// 7. Commit
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing fork transaction: %w", err)
	}

	// 8. Publish event
	session := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, session)

	return &ForkResult{
		NewSession:   session,
		CopiedCount:  len(msgsToCopy),
		ForkMarkerID: markerMsg.ID,
	}, nil
}

// marshalForkMarker creates a JSON parts array containing the fork marker
// as a text part. The format matches the message Parts column schema.
func marshalForkMarker(content string) string {
	escaped := strings.ReplaceAll(content, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return fmt.Sprintf(`[{"type":"text","text":"%s"}]`, escaped)
}
