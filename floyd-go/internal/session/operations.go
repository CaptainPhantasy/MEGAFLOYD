package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

var ErrInvalidUndo = errors.New("undo count exceeds available user turns")

// MetadataMutator updates only session metadata under one database transaction.
// Callbacks must not access the database while the transaction is held.
type MetadataMutator interface {
	MutateMetadata(context.Context, string, func(map[string]any) error) (Session, error)
}

func (s *service) MutateMetadata(ctx context.Context, id string, mutate func(map[string]any) error) (Session, error) {
	return s.updateMetadata(ctx, id, nil, mutate)
}

func (s *service) UpdateMetadata(ctx context.Context, id string, title *string, patch map[string]any) (Session, error) {
	return s.updateMetadata(ctx, id, title, func(metadata map[string]any) error {
		for key, value := range patch {
			metadata[key] = value
		}
		return nil
	})
}

func (s *service) updateMetadata(ctx context.Context, id string, title *string, mutate func(map[string]any) error) (Session, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	stored, err := qtx.GetSessionByID(ctx, id)
	if err != nil {
		return Session{}, err
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(stored.Metadata), &metadata); err != nil {
		return Session{}, fmt.Errorf("decode session metadata: %w", err)
	}
	if err := mutate(metadata); err != nil {
		return Session{}, err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return Session{}, fmt.Errorf("encode session metadata: %w", err)
	}
	if err := qtx.UpdateSessionMetadata(ctx, db.UpdateSessionMetadataParams{Metadata: string(encoded), ID: id}); err != nil {
		return Session{}, err
	}
	if title != nil {
		if err := qtx.RenameSession(ctx, db.RenameSessionParams{Title: *title, ID: id}); err != nil {
			return Session{}, err
		}
	}
	updated, err := qtx.GetSessionByID(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	result := s.fromDBItem(updated)
	s.applyEstimatedUsageState(&result)
	s.Publish(pubsub.UpdatedEvent, result)
	return result, nil
}

func (s *service) SetArchived(ctx context.Context, id string, archived bool) (Session, error) {
	at := sql.NullInt64{}
	if archived {
		at = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	}
	stored, err := s.q.SetSessionArchived(ctx, db.SetSessionArchivedParams{ID: id, ArchivedAt: at})
	if err != nil {
		return Session{}, err
	}
	result := s.fromDBItem(stored)
	s.applyEstimatedUsageState(&result)
	s.Publish(pubsub.UpdatedEvent, result)
	return result, nil
}

type UndoResult struct {
	Session           Session
	RemovedTurns      int
	RemovedMessageIDs []string
	RecordID          string
}

// Undo removes complete user turns from the active transcript. The transaction
// retains the removed rows and their file checkpoints in a recovery record.
// Files on disk and already incurred usage are not rewritten by transcript undo.
func (s *service) Undo(ctx context.Context, id string, count int) (*UndoResult, error) {
	if count < 1 {
		return nil, ErrInvalidUndo
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	stored, err := qtx.GetSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	messages, err := qtx.ListMessagesBySession(ctx, id)
	if err != nil {
		return nil, err
	}
	var starts []int
	for i, msg := range messages {
		if msg.Role == "user" {
			starts = append(starts, i)
		}
	}
	if count > len(starts) {
		return nil, ErrInvalidUndo
	}
	removed := messages[starts[len(starts)-count]:]
	recovery := struct {
		Session  db.Session           `json:"session"`
		Messages []db.Message         `json:"messages"`
		Turns    []db.SessionTurn     `json:"turns"`
		Files    []db.TurnFileHistory `json:"turn_files"`
	}{Session: stored, Messages: removed}
	result := &UndoResult{RemovedTurns: count, RecordID: uuid.NewString()}
	for _, msg := range removed {
		result.RemovedMessageIDs = append(result.RemovedMessageIDs, msg.ID)
		if msg.Role != "user" {
			continue
		}
		turn, err := qtx.GetSessionTurnByMessage(ctx, db.GetSessionTurnByMessageParams{SessionID: id, UserMessageID: msg.ID})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		recovery.Turns = append(recovery.Turns, turn)
		files, err := qtx.ListTurnFiles(ctx, db.ListTurnFilesParams{SessionID: id, UserMessageID: msg.ID})
		if err != nil {
			return nil, err
		}
		recovery.Files = append(recovery.Files, files...)
	}
	encoded, err := json.Marshal(recovery)
	if err != nil {
		return nil, err
	}
	if err := qtx.InsertSessionUndoRecord(ctx, db.InsertSessionUndoRecordParams{ID: result.RecordID, SessionID: id, Payload: string(encoded)}); err != nil {
		return nil, err
	}
	for _, msg := range removed {
		if err := qtx.DeleteMessage(ctx, msg.ID); err != nil {
			return nil, err
		}
	}
	if err := qtx.ClearSessionSummary(ctx, id); err != nil {
		return nil, err
	}
	updated, err := qtx.GetSessionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result.Session = s.fromDBItem(updated)
	s.applyEstimatedUsageState(&result.Session)
	s.Publish(pubsub.UpdatedEvent, result.Session)
	return result, nil
}
