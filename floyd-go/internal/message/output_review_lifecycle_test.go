package message

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/stretchr/testify/require"
)

func TestOutputReviewDeletionOwnsAllStoredRevisions(t *testing.T) {
	for _, removeSession := range []bool{false, true} {
		name := "message"
		if removeSession {
			name = "session"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			svc, conn, sid, dir := reviewedSQL(t, ReviewConfig{})
			q := db.New(conn)
			sessions := session.NewService(q, conn)
			other, err := sessions.Create(ctx, "unrelated history")
			require.NoError(t, err)
			create := func(id, text string) Message {
				m, e := svc.Create(ctx, id, CreateMessageParams{Role: Assistant, Parts: []ContentPart{TextContent{Text: text}}})
				require.NoError(t, e)
				m.AddFinish(FinishReasonEndTurn, "", "")
				require.NoError(t, svc.Update(ctx, m))
				return m
			}
			removed := create(sid, "History to remove.")
			kept := create(other.ID, "Unrelated history stays.")
			_, err = conn.ExecContext(ctx, `INSERT INTO output_review_corrections(message_id,revision,receipt) VALUES (?,1,'{}')`, removed.ID)
			require.NoError(t, err)
			_, err = conn.ExecContext(ctx, `INSERT INTO output_review_attempts(message_id,source_hash,candidate,issue) VALUES (?,'test','candidate','test')`, removed.ID)
			require.NoError(t, err)
			_, err = ReviewText(ctx, svc, sid, "suggestion", "Auxiliary display history.")
			require.NoError(t, err)
			var auxiliary string
			require.NoError(t, conn.QueryRowContext(ctx, `SELECT message_id FROM output_review_private WHERE session_id=? AND kind='suggestion' LIMIT 1`, sid).Scan(&auxiliary))

			if removeSession {
				require.NoError(t, sessions.Delete(ctx, sid))
			} else {
				require.NoError(t, svc.Delete(ctx, removed.ID))
			}
			assertGone := func(id string) {
				t.Helper()
				for _, table := range []string{"output_review_private", "output_review_public", "output_review_corrections", "output_review_attempts", "output_review_events"} {
					var count int
					require.NoError(t, conn.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE message_id=?", id).Scan(&count))
					require.Zero(t, count, table)
				}
				_, e := PublicGet(ctx, svc, id)
				require.ErrorIs(t, e, sql.ErrNoRows, "deleted history must not be resurrected by the private fallback")
			}
			assertGone(removed.ID)
			if removeSession {
				assertGone(auxiliary)
			} else {
				_, err = FinalPublicText(ctx, svc, auxiliary)
				require.NoError(t, err, "deleting one message must retain unrelated auxiliary history")
			}
			visible, err := FinalPublicText(ctx, svc, kept.ID)
			require.NoError(t, err)
			require.Equal(t, kept.Content().Text, visible)
			require.Error(t, svc.Update(ctx, removed), "a late callback must not recreate deleted journal rows")
			assertGone(removed.ID)
			require.NoError(t, db.Release(dir))
			conn, err = db.Connect(ctx, dir)
			require.NoError(t, err)
			svc = NewService(db.New(conn), WithDB(conn), WithDebounce(0))
			require.NoError(t, EnableOutputReview(ctx, svc, ReviewConfig{}))
			assertGone(removed.ID)
			visible, err = FinalPublicText(ctx, svc, kept.ID)
			require.NoError(t, err)
			require.Equal(t, kept.Content().Text, visible)
		})
	}
}

func TestOutputReviewMigrationRetainsOwnedAuxiliaryCorrections(t *testing.T) {
	ctx := t.Context()
	svc, conn, sid, _ := reviewedSQL(t, ReviewConfig{})
	for _, trigger := range []string{"message_delete", "session_delete", "private_owner", "public_owner", "event_owner", "correction_owner", "attempt_owner"} {
		_, err := conn.ExecContext(ctx, "DROP TRIGGER output_review_"+trigger)
		require.NoError(t, err)
	}
	removed, err := svc.Create(ctx, sid, CreateMessageParams{Role: Assistant, Parts: []ContentPart{TextContent{Text: "Deleted before the upgrade."}}})
	require.NoError(t, err)
	removed.AddFinish(FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(ctx, removed))
	// The old schema left these journal rows behind after a real deletion.
	require.NoError(t, db.New(conn).DeleteMessage(ctx, removed.ID))
	_, err = ReviewText(ctx, svc, sid, "suggestion", "Retained auxiliary display.")
	require.NoError(t, err)
	var auxiliary string
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT message_id FROM output_review_private WHERE session_id=? AND kind='suggestion' LIMIT 1`, sid).Scan(&auxiliary))
	// Older correction code mislabeled auxiliary revisions as answers.
	_, err = conn.ExecContext(ctx, `INSERT INTO output_review_private(message_id,session_id,revision,kind,source_hash,parts) SELECT message_id,session_id,revision+1,'answer',source_hash,parts FROM output_review_private WHERE message_id=?`, auxiliary)
	require.NoError(t, err)
	orphan, err := session.NewService(db.New(conn), conn).Create(ctx, "deleted auxiliary owner")
	require.NoError(t, err)
	_, err = ReviewText(ctx, svc, orphan.ID, "suggestion", "Deleted auxiliary display.")
	require.NoError(t, err)
	require.NoError(t, db.New(conn).DeleteSession(ctx, orphan.ID))
	up, err := db.FS.ReadFile("migrations/20261025104000_output_review_lifecycle.sql")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, strings.Split(string(up), "-- +goose Down")[0])
	require.NoError(t, err, "execute the real upgrade over the old schema's retained rows")
	_, err = PublicGet(ctx, svc, removed.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	var retained, abandoned int
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT count(*) FROM output_review_private WHERE message_id=?`, auxiliary).Scan(&retained))
	require.Equal(t, 2, retained, "retain the auxiliary original and its historical correction")
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT count(*) FROM output_review_private WHERE session_id=?`, orphan.ID).Scan(&abandoned))
	require.Zero(t, abandoned)
	text, err := FinalPublicText(ctx, svc, auxiliary)
	require.NoError(t, err)
	require.Equal(t, "Retained auxiliary display.", text)
}
