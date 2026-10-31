package errors

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserErrorKeepsCauseAndRecoveryInstructions(t *testing.T) {
	t.Parallel()
	cause := stderrors.New("connection refused")
	err := CreateUserError("Service unavailable", UserErrorOptions{Category: ErrorCategoryConnection, Cause: cause, Resolution: []string{"Start the service.", "Retry the request."}, Details: map[string]any{"attempt": 2}})
	require.ErrorIs(t, err, cause)
	require.Equal(t, "Service unavailable (category=Connection)", err.String())
	require.Equal(t, "Error: Service unavailable\n\nTo resolve this:\n- Start the service.\n- Retry the request.\n\nDetails:\nattempt: 2", FormatErrorForDisplay(err))
	require.Same(t, err, EnsureUserError(err, "fallback", UserErrorOptions{}))
	wrapped := EnsureUserError(cause, "fallback", UserErrorOptions{})
	require.ErrorIs(t, wrapped, cause)
	require.Equal(t, "connection refused", wrapped.Message())
	require.Empty(t, FormatErrorForDisplay(nil))
}

func TestSystemErrorDebugDetailsAreOptIn(t *testing.T) {
	t.Setenv("DEBUG", "false")
	require.Equal(t, "System error: unavailable", FormatErrorForDisplay(stderrors.New("unavailable")))
	t.Setenv("DEBUG", "true")
	require.Contains(t, FormatErrorForDisplay(stderrors.New("unavailable")), "Stack trace:")
}
