package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/filelock"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

func TestPermissionObserverWritesReservedTarget(t *testing.T) {
	for _, grant := range []bool{true, false} {
		name := "deny"
		if grant {
			name = "grant"
		}
		t.Run(name, func(t *testing.T) {
			env := testEnv(t)
			env.permissions.SetSkipRequests(false)
			session, err := env.sessions.Create(t.Context(), "permission lock regression")
			require.NoError(t, err)
			cfg := &config.Config{Hooks: map[string][]config.HookConfig{
				hooks.EventPermissionRequest:  {{Command: `printf requested >> target.txt; printf '{"decision":"allow"}'`, Timeout: 5}},
				hooks.EventPermissionResolved: {{Command: `printf resolved >> target.txt; printf '{"decision":"allow"}'`, Timeout: 5}},
			}}
			runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, env.workingDir), nil)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = runtime.Close(ctx)
			}()
			env.permissions.(interface {
				SetLifecycleObserver(func(context.Context, permission.Observation))
			}).SetLifecycleObserver(runtime.ObservePermission)
			tool := tools.NewWriteTool(nil, env.permissions, env.history, *env.filetracker, env.workingDir)
			wrapped := wrapToolsWithRuntimeObservation(wrapToolsWithRuntimePre([]fantasy.AgentTool{tool}, runtime), runtime)[0]
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), tools.SessionIDContextKey, session.ID))
			defer cancel()
			requests := env.permissions.Subscribe(ctx)
			input, err := json.Marshal(tools.WriteParams{FilePath: "target.txt", Content: "parent"})
			require.NoError(t, err)
			type result struct {
				response fantasy.ToolResponse
				err      error
			}
			done := make(chan result, 1)
			go func() {
				response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "reserved-write", Name: "write", Input: string(input)})
				done <- result{response, err}
			}()
			var request permission.PermissionRequest
			select {
			case event := <-requests:
				request = event.Payload
			case <-time.After(750 * time.Millisecond):
				t.Fatal("permission observer deadlocked on the parent write reservation")
			}
			target, err := filelock.Resolve(env.workingDir, "target.txt")
			require.NoError(t, err)
			data, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, "requested", string(data))
			select {
			case <-done:
				t.Fatal("observer allow must not grant the native permission")
			default:
			}
			otherCtx, otherCancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			release, lockErr := filelock.Acquire(otherCtx, []string{target})
			otherCancel()
			if release != nil {
				release()
			}
			require.ErrorIs(t, lockErr, context.DeadlineExceeded, "unrelated writers must remain excluded while permission is pending")
			if grant {
				require.True(t, env.permissions.Grant(request))
			} else {
				require.True(t, env.permissions.Deny(request))
			}
			select {
			case result := <-done:
				require.NoError(t, result.err)
				require.Equal(t, !grant, result.response.IsError, result.response.Content)
			case <-time.After(time.Second):
				t.Fatal("permission resolution observer deadlocked on the parent reservation")
			}
			data, err = os.ReadFile(target)
			require.NoError(t, err)
			if grant {
				require.Equal(t, "parent", string(data))
			} else {
				require.Equal(t, "requestedresolved", string(data))
			}
			release, err = filelock.Acquire(t.Context(), []string{target})
			require.NoError(t, err)
			release()
		})
	}
}

func TestPermissionAsyncObserverWaitsForReservedTarget(t *testing.T) {
	env := testEnv(t)
	env.permissions.SetSkipRequests(false)
	session, err := env.sessions.Create(t.Context(), "async permission lock regression")
	require.NoError(t, err)
	cfg := &config.Config{Hooks: map[string][]config.HookConfig{
		hooks.EventPermissionRequest: {{Command: `printf observer >> target.txt; printf '{}'`, Timeout: 5, Async: true}},
	}}
	runtime := hookruntime.New(t.Context(), config.NewTestStoreWithWorkingDir(cfg, env.workingDir), nil)
	defer runtime.Close(context.Background())
	env.permissions.(interface {
		SetLifecycleObserver(func(context.Context, permission.Observation))
	}).SetLifecycleObserver(runtime.ObservePermission)
	tool := tools.NewWriteTool(nil, env.permissions, env.history, *env.filetracker, env.workingDir)
	wrapped := wrapToolsWithRuntimeObservation(wrapToolsWithRuntimePre([]fantasy.AgentTool{tool}, runtime), runtime)[0]
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), tools.SessionIDContextKey, session.ID))
	defer cancel()
	requests := env.permissions.Subscribe(ctx)
	done := make(chan error, 1)
	go func() {
		response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "async-reserved-write", Name: "write", Input: `{"file_path":"target.txt","content":"parent"}`})
		if err == nil && response.IsError {
			err = errors.New(response.Content)
		}
		done <- err
	}()
	var request permission.PermissionRequest
	select {
	case event := <-requests:
		request = event.Payload
	case <-time.After(time.Second):
		t.Fatal("async observer blocked permission")
	}
	target := filepath.Join(env.workingDir, "target.txt")
	require.Never(t, func() bool { _, err := os.Stat(target); return err == nil }, 75*time.Millisecond, 5*time.Millisecond, "async observers must not inherit a synchronous reservation loan")
	require.True(t, env.permissions.Grant(request))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("native write did not complete")
	}
	require.Eventually(t, func() bool { data, err := os.ReadFile(target); return err == nil && string(data) == "parentobserver" }, time.Second, 5*time.Millisecond)
}
