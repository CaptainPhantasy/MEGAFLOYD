package session

import (
	"context"
	"database/sql"
)

// cloneOutputReview preserves the private/provider and public/display split
// inside the same transaction as the copied conversation.
func cloneOutputReview(ctx context.Context, tx *sql.Tx, sourceID, destinationID string, ids map[string]string) error {
	for oldID, newID := range ids {
		for _, query := range []string{
			"INSERT INTO output_review_private(message_id,session_id,revision,kind,source_hash,parts) SELECT ?,?,revision,kind,source_hash,parts FROM output_review_private WHERE message_id=?",
			"INSERT INTO output_review_public(message_id,session_id,revision,source_hash,parts,state,receipt) SELECT ?,?,revision,source_hash,parts,state,receipt FROM output_review_public WHERE message_id=?",
		} {
			if _, err := tx.ExecContext(ctx, query, newID, destinationID, oldID); err != nil {
				return err
			}
		}
		for _, query := range []string{
			"INSERT INTO output_review_corrections(message_id,revision,receipt) SELECT ?,revision,receipt FROM output_review_corrections WHERE message_id=?",
			"INSERT INTO output_review_attempts(message_id,source_hash,candidate,issue) SELECT ?,source_hash,candidate,issue FROM output_review_attempts WHERE message_id=?",
		} {
			if _, err := tx.ExecContext(ctx, query, newID, oldID); err != nil {
				return err
			}
		}
	}
	type event struct {
		messageID, parts, state string
		revision                int64
	}
	var events []event
	rows, err := tx.QueryContext(ctx, "SELECT message_id,revision,parts,state FROM output_review_events WHERE session_id=? ORDER BY cursor", sourceID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var record event
		if err := rows.Scan(&record.messageID, &record.revision, &record.parts, &record.state); err != nil {
			rows.Close()
			return err
		}
		if _, ok := ids[record.messageID]; ok {
			events = append(events, record)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, record := range events {
		if _, err := tx.ExecContext(ctx, "INSERT INTO output_review_events(message_id,session_id,revision,parts,state) VALUES(?,?,?,?,?)",
			ids[record.messageID], destinationID, record.revision, record.parts, record.state); err != nil {
			return err
		}
	}
	return nil
}
