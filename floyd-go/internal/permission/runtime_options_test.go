package permission

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAllowlistDoesNotEnableOtherTools(t *testing.T) {
	t.Parallel()
	s := NewPermissionService(t.TempDir(), false, nil)
	s.(RuntimeAllowlist).SetAllowedTools([]string{"view"})
	allowed, err := s.Request(context.Background(), CreatePermissionRequest{ToolName: "view"})
	require.NoError(t, err)
	require.True(t, allowed)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	allowed, err = s.Request(ctx, CreatePermissionRequest{ToolName: "bash"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, allowed)
	s.(RuntimeAllowlist).SetAllowedTools([]string{})
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	allowed, err = s.Request(ctx, CreatePermissionRequest{ToolName: "view"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, allowed)
}

func TestNonInteractiveRequiresExplicitPolicyGrant(t *testing.T) {
	s := NewPermissionService(t.TempDir(), false, nil)
	ctx := WithNonInteractive(context.Background())
	events := s.Subscribe(ctx)
	granted, err := s.Request(ctx, CreatePermissionRequest{ToolName: "bash", SessionID: "s"})
	require.False(t, granted)
	require.ErrorIs(t, err, ErrInteractionRequired)
	select {
	case <-events:
		t.Fatal("unanswerable permission prompt was published")
	default:
	}
	s.(RuntimeAllowlist).SetAllowedTools([]string{"view"})
	granted, err = s.Request(ctx, CreatePermissionRequest{ToolName: "view", SessionID: "s"})
	require.True(t, granted)
	require.NoError(t, err)
	granted, err = s.Request(ctx, CreatePermissionRequest{ToolName: "bash", SessionID: "s"})
	require.False(t, granted)
	require.ErrorIs(t, err, ErrInteractionRequired)
	require.NoError(t, s.(ModeService).SetMode(ModeAuto))
	granted, err = s.Request(ctx, CreatePermissionRequest{ToolName: "bash", SessionID: "s"})
	require.True(t, granted)
	require.NoError(t, err)
}
