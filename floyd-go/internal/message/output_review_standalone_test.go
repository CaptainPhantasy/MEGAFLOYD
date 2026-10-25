package message

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/legacy-ai/floyd/internal/db"
)

func TestOutputReviewStandaloneCorrectionRetainsAuxiliaryOwnership(t *testing.T) {
	svc, conn, sid, _ := reviewedSQL(t, ReviewConfig{})
	original := "Use `status --json` for the log."
	draft, err := BeginStandaloneReview(t.Context(), svc, sid, "suggestion", original)
	require.NoError(t, err)
	request, err := OutputReviewRequest(t.Context(), svc, draft.ID)
	require.NoError(t, err)
	response := ReviewResponse{
		SourceSHA256: request.SourceSHA256,
		Text:         "Read the log with `status --json`.",
		Disposition:  "publish",
		Reviewer:     "trusted-fixture-wording",
		Provenance:   []string{"fixture:standalone-correction"},
	}
	require.NoError(t, ReviewCandidate(t.Context(), svc, draft.ID, response))
	latest, err := db.New(conn).GetLatestOutputReviewPrivate(t.Context(), draft.ID)
	require.NoError(t, err)
	require.Equal(t, "suggestion", latest.Kind, "a correction must retain its actual auxiliary owner")
	_, err = svc.Get(t.Context(), draft.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "suggestions must not become provider conversation messages")
	public, err := FinalPublicText(t.Context(), svc, draft.ID)
	require.NoError(t, err)
	require.Equal(t, response.Text, public)
	var originalParts string
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT parts FROM output_review_private WHERE message_id=? AND revision=1", draft.ID).Scan(&originalParts))
	parts, err := unmarshalParts([]byte(originalParts))
	require.NoError(t, err)
	private := Message{Parts: parts}
	require.Equal(t, original, private.Content().Text)
}
