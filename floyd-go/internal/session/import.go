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

// ImportMessage is one message copied from an external session store. Parts
// are pre-serialized in the storage format so this package does not depend
// on the message package (which its tests would turn into a cycle).
type ImportMessage struct {
	Role      string
	Parts     []byte
	CreatedAt time.Time
}

// ImportResult describes an engine-session import.
type ImportResult struct {
	Session  Session
	Copied   int
	Existing bool
}

// EngineImporter is implemented by session services that can import a
// session produced by the Floyd Code web engine. It is intentionally not
// part of Service because client workspaces and test doubles cannot write
// the local database.
type EngineImporter interface {
	ImportEngine(ctx context.Context, id, title string, messages []ImportMessage) (*ImportResult, error)
}

// ImportEngine imports through svc when the underlying service supports it.
func ImportEngine(ctx context.Context, svc Service, id, title string, messages []ImportMessage) (*ImportResult, error) {
	importer, ok := svc.(EngineImporter)
	if !ok {
		return nil, fmt.Errorf("session service does not support engine import")
	}
	return importer.ImportEngine(ctx, id, title, messages)
}

// ImportEngine creates a local session with the engine's own ID so imports
// are idempotent: running it twice never duplicates a transcript.
func (s *service) ImportEngine(ctx context.Context, id, title string, messages []ImportMessage) (*ImportResult, error) {
	if id == "" {
		return nil, fmt.Errorf("engine session id is required")
	}
	if existing, err := s.Get(ctx, id); err == nil {
		return &ImportResult{Session: existing, Existing: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("look up session %s: %w", id, err)
	}
	if title == "" {
		title = "Imported engine session"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin import transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := s.q.WithTx(tx)
	dbSession, err := qtx.CreateSession(ctx, db.CreateSessionParams{
		ID:    id,
		Title: title,
	})
	if err != nil {
		return nil, fmt.Errorf("create imported session: %w", err)
	}

	for index, msg := range messages {
		stored, err := qtx.CreateMessage(ctx, db.CreateMessageParams{
			ID:        uuid.New().String(),
			SessionID: id,
			Role:      msg.Role,
			Parts:     string(msg.Parts),
		})
		if err != nil {
			return nil, fmt.Errorf("insert message %d: %w", index, err)
		}
		if msg.Role == "assistant" {
			parts, err := importedOutputParts(msg.Parts)
			if err != nil {
				return nil, fmt.Errorf("project imported message %d: %w", index, err)
			}
			if err := qtx.UpsertOutputReviewPublic(ctx, db.UpsertOutputReviewPublicParams{
				MessageID: stored.ID,
				SessionID: id,
				Parts:     parts,
				State:     "imported_visible",
				Receipt:   `{"reviewer":"imported-existing-history"}`,
			}); err != nil {
				return nil, fmt.Errorf("preserve imported display %d: %w", index, err)
			}
			if err := qtx.CreateOutputReviewEvent(ctx, db.CreateOutputReviewEventParams{
				MessageID: stored.ID,
				SessionID: id,
				Parts:     parts,
				State:     "imported_visible",
			}); err != nil {
				return nil, fmt.Errorf("preserve imported replay %d: %w", index, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import: %w", err)
	}

	imported := s.fromDBItem(dbSession)
	s.Publish(pubsub.CreatedEvent, imported)
	return &ImportResult{Session: imported, Copied: len(messages)}, nil
}

// importedOutputParts preserves imported display history without treating it
// as newly reviewed generation. Signed reasoning stays in the private row.
func importedOutputParts(parts []byte) (string, error) {
	var original []json.RawMessage
	if err := json.Unmarshal(parts, &original); err != nil {
		return "", err
	}
	public := make([]json.RawMessage, 0, len(original))
	for _, part := range original {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(part, &envelope); err != nil {
			return "", err
		}
		if envelope.Type != "reasoning" {
			public = append(public, part)
		}
	}
	encoded, err := json.Marshal(public)
	return string(encoded), err
}
