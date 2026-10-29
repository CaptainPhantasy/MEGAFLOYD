package session

import (
	"context"
	"database/sql"

	"github.com/legacy-ai/floyd/internal/db"
)

// cloneOutputReview preserves the private/provider and public/display split
// inside the same transaction as the copied conversation.
func cloneOutputReview(ctx context.Context, tx *sql.Tx, sourceID, destinationID string, ids map[string]string) error {
	q := db.New(tx)
	for oldID, newID := range ids {
		if err := q.CloneOutputReviewPrivate(ctx, db.CloneOutputReviewPrivateParams{NewMessageID: newID, DestinationSessionID: destinationID, SourceMessageID: oldID}); err != nil {
			return err
		}
		if err := q.CloneOutputReviewPublic(ctx, db.CloneOutputReviewPublicParams{NewMessageID: newID, DestinationSessionID: destinationID, SourceMessageID: oldID}); err != nil {
			return err
		}
		if err := q.CloneOutputReviewCorrections(ctx, db.CloneOutputReviewCorrectionsParams{NewMessageID: newID, SourceMessageID: oldID}); err != nil {
			return err
		}
		if err := q.CloneOutputReviewAttempts(ctx, db.CloneOutputReviewAttemptsParams{NewMessageID: newID, SourceMessageID: oldID}); err != nil {
			return err
		}
	}
	events, err := q.ListOutputReviewCloneEvents(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, record := range events {
		messageID, ok := ids[record.MessageID]
		if !ok {
			continue
		}
		if err := q.CreateOutputReviewCloneEvent(ctx, db.CreateOutputReviewCloneEventParams{MessageID: messageID, SessionID: destinationID, Revision: record.Revision, Parts: record.Parts, State: record.State}); err != nil {
			return err
		}
	}
	return nil
}
