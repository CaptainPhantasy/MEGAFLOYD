package agent

import (
	"context"
	"testing"

	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

func TestProtectedDeletionRequiresInteractiveConfirmation(t *testing.T) {
	allowed, err := (questionConfirmer{}).Confirm(permission.WithNonInteractive(context.Background()), deletionguard.ConfirmationRequest{})
	require.False(t, allowed)
	require.ErrorIs(t, err, permission.ErrInteractionRequired)
}

func TestQueuedCallContextRestoresOwnInteractionAndCorrelation(t *testing.T) {
	old := permission.WithNonInteractive(WithRunID(context.Background(), "old"))
	interactive := acceptedCallContext(old, SessionAgentCall{RunID: "new"})
	require.False(t, permission.IsNonInteractive(interactive))
	require.Equal(t, "new", RunIDFromContext(interactive))
	queued := acceptedCallContext(interactive, SessionAgentCall{RunID: "queued", NoHumanInteraction: true})
	require.True(t, permission.IsNonInteractive(queued))
	require.Equal(t, "queued", RunIDFromContext(queued))
}
