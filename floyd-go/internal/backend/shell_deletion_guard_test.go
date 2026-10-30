package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hookruntime"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/stretchr/testify/require"
)

func shellGuardWorkspace(t *testing.T) (*Backend, *Workspace) {
	t.Helper()
	b, _ := newTestBackend(t)
	ws, _ := insertTestWorkspace(t, b, t.TempDir())
	ws.App = &app.App{}
	require.NoError(t, os.WriteFile(filepath.Join(ws.Path, "fixture"), []byte("preserved fixture bytes"), 0o600))
	return b, ws
}

func TestBackendShellRejectsNestedDestructionBeforeLaunch(t *testing.T) {
	b, ws := shellGuardWorkspace(t)
	// The false branch never removes anything, even on the vulnerable baseline.
	_, err := b.RunShellCommand(t.Context(), ws.ID, proto.ShellCommandRequest{Command: "if false; then rm fixture; fi; printf launched > marker"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "quarantine")
	_, statErr := os.Stat(filepath.Join(ws.Path, "marker"))
	require.ErrorIs(t, statErr, os.ErrNotExist, "no command body may run before the denial")
	bytes, readErr := os.ReadFile(filepath.Join(ws.Path, "fixture"))
	require.NoError(t, readErr)
	require.Equal(t, "preserved fixture bytes", string(bytes))
}

func TestBackendShellPreservesHarmlessAndQuarantineCommands(t *testing.T) {
	b, ws := shellGuardWorkspace(t)
	response, err := b.RunShellCommand(t.Context(), ws.ID, proto.ShellCommandRequest{Command: "printf usable"})
	require.NoError(t, err)
	require.Zero(t, response.ExitCode)
	require.Equal(t, "usable", response.Output)
	response, err = b.RunShellCommand(t.Context(), ws.ID, proto.ShellCommandRequest{Command: "mv fixture quarantined-fixture"})
	require.NoError(t, err)
	require.Zero(t, response.ExitCode)
	bytes, err := os.ReadFile(filepath.Join(ws.Path, "quarantined-fixture"))
	require.NoError(t, err)
	require.Equal(t, "preserved fixture bytes", string(bytes))
}

func TestBackendShellChecksCommandAfterHookRewrite(t *testing.T) {
	b, ws := shellGuardWorkspace(t)
	rewritten := "if false; then rm fixture; fi; printf launched > marker"
	payload, err := json.Marshal(map[string]any{"decision": "allow", "updated_input": map[string]string{"command": rewritten}})
	require.NoError(t, err)
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{DataDirectory: t.TempDir()}, Hooks: map[string][]config.HookConfig{hooks.EventPreToolUse: {{Command: "printf '%s\\n' '" + string(payload) + "'"}}}}, ws.Path)
	require.NoError(t, cfg.Config().ValidateHooks())
	runtime := hookruntime.New(t.Context(), cfg, nil)
	t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })
	ws.Hooks = runtime
	_, err = b.RunShellCommand(t.Context(), ws.ID, proto.ShellCommandRequest{Command: "printf harmless-original"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "quarantine")
	_, statErr := os.Stat(filepath.Join(ws.Path, "marker"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	bytes, readErr := os.ReadFile(filepath.Join(ws.Path, "fixture"))
	require.NoError(t, readErr)
	require.Equal(t, "preserved fixture bytes", string(bytes))
}
