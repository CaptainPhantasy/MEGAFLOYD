package permission

import (
	"context"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"
	"github.com/stretchr/testify/require"
)

type pendingPermissionResult struct {
	granted bool
	err     error
}

func awaitPendingPermission(t *testing.T, events <-chan pubsub.Event[PermissionRequest]) PermissionRequest {
	t.Helper()
	select {
	case event := <-events:
		return event.Payload
	case <-time.After(3 * time.Second):
		t.Fatal("permission request was not published")
		return PermissionRequest{}
	}
}

func TestPendingPermissionHandoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	dir := t.TempDir()
	service := NewPermissionService(dir, false, nil)
	firstInterface := service.Subscribe(ctx)
	result := make(chan pendingPermissionResult, 1)
	go func() {
		granted, err := service.Request(ctx, CreatePermissionRequest{
			SessionID: "shared-session", ToolCallID: "shared-tool-call",
			ToolName: "bash", Action: "execute", Path: dir,
		})
		result <- pendingPermissionResult{granted, err}
	}()
	original := awaitPendingPermission(t, firstInterface)

	service.Subscribe(ctx)
	notifications := service.SubscribeNotifications(ctx)
	pending := service.Pending()
	require.Len(t, pending, 1)
	require.Equal(t, original, pending[0])
	require.True(t, service.Grant(pending[0]))
	require.Empty(t, service.Pending())
	require.False(t, service.Grant(original))

	select {
	case outcome := <-result:
		require.NoError(t, outcome.err)
		require.True(t, outcome.granted)
	case <-ctx.Done():
		t.Fatal("grant did not release the original request")
	}
	select {
	case notification := <-notifications:
		require.Equal(t, original.ToolCallID, notification.Payload.ToolCallID)
		require.True(t, notification.Payload.Granted)
	case <-ctx.Done():
		t.Fatal("late observer did not receive the resolution")
	}
}

func TestPendingPermissionCancellationIsNotStale(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	service := NewPermissionService(t.TempDir(), false, nil)
	events := service.Subscribe(t.Context())
	notifications := service.SubscribeNotifications(t.Context())
	result := make(chan pendingPermissionResult, 1)
	go func() {
		granted, err := service.Request(ctx, CreatePermissionRequest{
			SessionID: "shared-session", ToolCallID: "canceled-call",
			ToolName: "bash", Action: "execute", Path: ".",
		})
		result <- pendingPermissionResult{granted, err}
	}()
	request := awaitPendingPermission(t, events)
	require.Len(t, service.Pending(), 1)
	select {
	case notification := <-notifications:
		require.Equal(t, request.ToolCallID, notification.Payload.ToolCallID)
		require.False(t, notification.Payload.Granted)
		require.False(t, notification.Payload.Denied)
	case <-time.After(3 * time.Second):
		t.Fatal("pending permission notification was not published")
	}
	cancel()
	select {
	case outcome := <-result:
		require.ErrorIs(t, outcome.err, context.Canceled)
		require.False(t, outcome.granted)
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not release permission request")
	}
	require.Empty(t, service.Pending())
	select {
	case notification := <-notifications:
		require.Equal(t, request.ToolCallID, notification.Payload.ToolCallID)
		require.False(t, notification.Payload.Granted)
		require.True(t, notification.Payload.Denied)
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not publish a terminal notification")
	}
	require.False(t, service.Grant(request))
	require.False(t, service.GrantPersistent(request))
	require.False(t, service.Deny(request))
	select {
	case notification := <-notifications:
		t.Fatalf("cancellation published a duplicate notification: %+v", notification.Payload)
	default:
	}
}
