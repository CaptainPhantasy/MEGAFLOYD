package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/update"
	"github.com/stretchr/testify/require"
)

type hookNoUpdateClient struct {
	called chan struct{}
	once   sync.Once
}

func (c *hookNoUpdateClient) Latest(context.Context) (*update.Release, error) {
	c.once.Do(func() { close(c.called) })
	return nil, errors.New("fixture has no update service")
}

func TestModernHooksAppPermissionsAndShutdown(t *testing.T) {
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	t.Setenv("FLOYD_HOME", t.TempDir())
	t.Setenv("MEGAFLOYD_POSTHOG_KEY", "")
	t.Setenv("HERDR_PANE_ID", "")
	noUpdate := &hookNoUpdateClient{called: make(chan struct{})}
	previous := update.Default
	update.Default = noUpdate
	defer func() { update.Default = previous }()
	dataDir := t.TempDir()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	cfg := &config.Config{Providers: csync.NewMap[string, config.ProviderConfig](), Options: &config.Options{DataDirectory: dataDir}, Hooks: map[string][]config.HookConfig{
		hooks.EventPermissionRequest:  {{Command: `echo '{"decision":"allow"}'`}},
		hooks.EventPermissionResolved: {{Command: `echo '{"decision":"allow"}'`}},
		hooks.EventActionRejected:     {{Command: `echo '{}'`}},
		hooks.EventPostToolUse:        {{Command: `sleep 30`, Async: true}},
	}}
	require.NoError(t, cfg.ValidateHooks())
	store := config.NewTestStoreWithWorkingDir(cfg, t.TempDir())
	app, err := New(t.Context(), conn, store, nil)
	require.NoError(t, err)
	shutdown := false
	defer func() {
		if !shutdown {
			app.Shutdown()
		}
	}()
	require.NotNil(t, app.Hooks)
	require.NotNil(t, app.Hooks.Store())
	select {
	case <-noUpdate.called:
	case <-time.After(2 * time.Second):
		t.Fatal("update fixture not used")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	requestEvents := app.Permissions.Subscribe(ctx)
	done := make(chan bool, 1)
	go func() {
		response, err := app.Hooks.ExecuteTool(ctx, "session-1", fantasy.ToolCall{ID: "permission-call", Name: "bash", Input: `{"command":"echo permission-call"}`}, func(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			granted, err := app.Permissions.Request(ctx, permission.CreatePermissionRequest{SessionID: "session-1", ToolCallID: "permission-call", ToolName: "bash", Action: "execute", Path: store.WorkingDir()})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return fantasy.NewTextErrorResponse("denied"), nil
			}
			return fantasy.NewTextResponse("granted"), nil
		})
		done <- err == nil && response.IsError
	}()
	var request permission.PermissionRequest
	select {
	case event := <-requestEvents:
		request = event.Payload
	case <-time.After(2 * time.Second):
		t.Fatal("permission request was not published: an observer must not approve, and the guarded tool body must run")
	}
	require.Equal(t, "permission-call", request.ToolCallID)
	select {
	case <-done:
		t.Fatal("observer result incorrectly resolved the real permission")
	default:
	}
	require.True(t, app.Permissions.Deny(request))
	select {
	case denied := <-done:
		require.True(t, denied)
	case <-time.After(2 * time.Second):
		t.Fatal("native denial did not complete")
	}
	page, err := app.Hooks.Store().List(t.Context(), hookstore.Query{ToolCallID: "permission-call"})
	require.NoError(t, err)
	outcomes := map[string]string{}
	for _, record := range page.Records {
		if record.State == "succeeded" {
			outcomes[record.Event] = record.Outcome
		}
	}
	require.Equal(t, "requested", outcomes[hooks.EventPermissionRequest])
	require.Equal(t, "denied", outcomes[hooks.EventPermissionResolved])
	require.Equal(t, "rejected", outcomes[hooks.EventActionRejected])

	// An async observer is still running when real App.Shutdown releases the DB.
	response, err := app.Hooks.ExecuteTool(t.Context(), "session-1", fantasy.ToolCall{ID: "async-call", Name: "view", Input: `{}`}, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("done"), nil
	})
	require.NoError(t, err)
	require.False(t, response.IsError)
	require.Eventually(t, func() bool {
		p, e := app.Hooks.Store().List(t.Context(), hookstore.Query{ToolCallID: "async-call"})
		return e == nil && len(p.Records) > 0
	}, 2*time.Second, 5*time.Millisecond)
	app.Shutdown()
	shutdown = true
	reader, closer, err := hookstore.OpenReader(dataDir)
	require.NoError(t, err)
	defer closer.Close()
	page, err = reader.List(t.Context(), hookstore.Query{ToolCallID: "async-call"})
	require.NoError(t, err)
	require.Len(t, page.Records, 2)
	require.Equal(t, "running", page.Records[0].State)
	require.Equal(t, "cancelled", page.Records[1].State)
	require.NotEmpty(t, page.Records[1].JobID)
	require.Equal(t, page.Records[0].ID, page.Records[1].ID)
}
