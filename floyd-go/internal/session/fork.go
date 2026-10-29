package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// ForkResult contains the result of a session fork operation.
type ForkResult struct {
	NewSession   Session
	CopiedCount  int
	ForkMarkerID string
}

// Forker is optional because remote workspaces cannot fork their DB locally.
type Forker interface {
	Fork(context.Context, string, int) (*ForkResult, error)
}

func Fork(ctx context.Context, svc Service, sourceSessionID string, upToMessage int) (*ForkResult, error) {
	forker, ok := svc.(Forker)
	if !ok {
		return nil, fmt.Errorf("session service does not support forking")
	}
	return forker.Fork(ctx, sourceSessionID, upToMessage)
}

// Fork copies through the inclusive zero-based message index; -1 copies all.
func (s *service) Fork(ctx context.Context, sourceID string, upToMessage int) (*ForkResult, error) {
	return s.clone(ctx, sourceID, upToMessage, nil, nil, false)
}

// Clone creates either a full transcript fork or an empty child branch.
func (s *service) Clone(ctx context.Context, sourceID string, title *string, metadata map[string]any, branch bool) (Session, error) {
	result, err := s.clone(ctx, sourceID, -1, title, metadata, branch)
	if err != nil {
		return Session{}, err
	}
	return result.NewSession, nil
}

func (s *service) clone(ctx context.Context, sourceID string, upToMessage int, title *string, patch map[string]any, branch bool) (*ForkResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	source, err := qtx.GetSessionByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("source session: %w", err)
	}
	metadata := make(map[string]any)
	if !branch {
		if err := json.Unmarshal([]byte(source.Metadata), &metadata); err != nil {
			return nil, err
		}
	}
	for key, value := range patch {
		metadata[key] = value
	}
	// Goals own execution IDs and consumed budgets; a new conversation does not inherit an active run.
	delete(metadata, "goal")
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	name := "Fork of " + source.Title
	if branch {
		name = "Branch of " + source.Title
	}
	if title != nil {
		name = *title
	}
	newID := uuid.NewString()
	if _, err := qtx.CreateSession(ctx, db.CreateSessionParams{ID: newID, ParentSessionID: sql.NullString{String: sourceID, Valid: true}, Title: name}); err != nil {
		return nil, err
	}
	if err := qtx.MarkSessionBranch(ctx, db.MarkSessionBranchParams{ID: newID, Metadata: string(encoded)}); err != nil {
		return nil, err
	}
	result := &ForkResult{}
	if !branch {
		messages, err := qtx.ListMessagesBySession(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		if upToMessage >= 0 && upToMessage < len(messages) {
			messages = messages[:upToMessage+1]
		}
		ids := make(map[string]string, len(messages))
		for _, msg := range messages {
			id := uuid.NewString()
			_, err := qtx.CopySessionMessage(ctx, db.CopySessionMessageParams{ID: id, SessionID: newID, Role: msg.Role, Parts: msg.Parts, Model: msg.Model, Provider: msg.Provider, IsSummaryMessage: msg.IsSummaryMessage, CreatedAt: msg.CreatedAt, UpdatedAt: msg.UpdatedAt, FinishedAt: msg.FinishedAt})
			if err != nil {
				return nil, err
			}
			ids[msg.ID] = id
		}
		if err := cloneOutputReview(ctx, tx, sourceID, newID, ids); err != nil {
			return nil, err
		}
		if err := history.CloneTurns(ctx, qtx, sourceID, newID, ids); err != nil {
			return nil, err
		}
		summaryID := ids[source.SummaryMessageID.String]
		if _, err := qtx.UpdateSession(ctx, db.UpdateSessionParams{ID: newID, Title: name, SummaryMessageID: sql.NullString{String: summaryID, Valid: summaryID != ""}, Todos: source.Todos}); err != nil {
			return nil, err
		}
		marker := fmt.Sprintf("[Fork] This session was forked from session %s at message index %d. Original title: %s", sourceID, len(messages)-1, source.Title)
		saved, err := qtx.CreateMessage(ctx, db.CreateMessageParams{ID: uuid.NewString(), SessionID: newID, Role: "system", Parts: marshalForkMarker(marker)})
		if err != nil {
			return nil, err
		}
		result.CopiedCount = len(messages)
		result.ForkMarkerID = saved.ID
	}
	stored, err := qtx.GetSessionByID(ctx, newID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result.NewSession = s.fromDBItem(stored)
	s.Publish(pubsub.CreatedEvent, result.NewSession)
	return result, nil
}

func marshalForkMarker(content string) string {
	encoded, _ := json.Marshal([]map[string]any{{"type": "text", "data": map[string]string{"text": content}}})
	return string(encoded)
}
