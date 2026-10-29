package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/hookstore"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestHooksInspectionCommandsRegistered(t *testing.T) {
	for _, name := range []string{"explain", "trace"} {
		command, _, err := rootCmd.Find([]string{"hooks", name})
		require.NoError(t, err)
		require.Equal(t, name, command.Name())
	}
	registered := map[string]bool{}
	for _, tool := range tools.GetBuiltinTools() {
		registered[tool.Name] = true
	}
	require.True(t, registered["hook_explain"])
	require.True(t, registered["hook_trace"])
}

func TestHooksExplainSnapshotDoesNotExecuteConfigOrHooks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	file := filepath.Join(root, "snapshot.json")
	cfg := config.Config{Hooks: map[string][]config.HookConfig{
		"pre_tool_use": {{Command: "touch '" + marker + "'", Matcher: "^bash$"}},
	}}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "floydrc"), []byte("touch '"+marker+"'"), 0o600))
	command, output := hooksTestCommand(root, "")
	command.SetArgs([]string{"hooks", "explain", "PreToolUse", "--config", file, "--tool", "bash", "--json"})
	require.NoError(t, command.Execute())
	var explanation tools.HookExplanation
	require.NoError(t, json.Unmarshal(output.Bytes(), &explanation))
	require.Equal(t, "json_snapshot", explanation.Source)
	require.Len(t, explanation.Entries, 1)
	require.True(t, explanation.Entries[0].Matched)
	require.NotContains(t, output.String(), marker)
	_, err = os.Stat(marker)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHooksExplainUsesOnlyExistingWorkspaceReads(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/v1/workspaces":
			_ = json.NewEncoder(w).Encode([]proto.Workspace{{ID: "selected", Path: cwd, DataDir: cwd}})
		case "/v1/workspaces/selected/config":
			_, _ = w.Write([]byte(`{"providers":{"private":{"api_key":"must-stay-private"}},"hooks":{"PreToolUse":[{"name":"check","command":"echo command-secret","matcher":"^bash$"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	command, output := hooksTestCommand(cwd, "tcp://"+strings.TrimPrefix(server.URL, "http://"))
	command.SetArgs([]string{"hooks", "explain", "PreToolUse", "--tool", "bash", "--json"})
	require.NoError(t, command.Execute())
	require.NotContains(t, output.String(), "must-stay-private")
	require.NotContains(t, output.String(), "command-secret")
	var explanation tools.HookExplanation
	require.NoError(t, json.Unmarshal(output.Bytes(), &explanation))
	require.Equal(t, "effective_runtime", explanation.Source)
	require.Len(t, explanation.Entries, 1)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"GET /v1/workspaces", "GET /v1/workspaces/selected/config"}, requests)
}

func TestHooksTraceReadsExistingDatabaseWithoutCreatingOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	store := hookstore.New(conn)
	for _, id := range []string{"one", "two"} {
		require.NoError(t, store.Append(ctx, hookstore.Record{ID: id, EventID: "event", Event: "PostToolUse", SessionID: "session", State: "completed"}))
	}
	require.NoError(t, db.Release(dataDir))
	command, output := hooksTestCommand(t.TempDir(), "")
	command.SetArgs([]string{"--data-dir", dataDir, "hooks", "trace", "--session", "session", "--limit", "1", "--json"})
	require.NoError(t, command.Execute())
	var page hookstore.Page
	require.NoError(t, json.Unmarshal(output.Bytes(), &page))
	require.Len(t, page.Records, 1)
	require.True(t, page.HasMore)
	missing := filepath.Join(t.TempDir(), "missing")
	command, output = hooksTestCommand(t.TempDir(), "")
	command.SetArgs([]string{"--data-dir", missing, "hooks", "trace", "--json"})
	require.NoError(t, command.Execute())
	require.NoError(t, json.Unmarshal(output.Bytes(), &page))
	require.Empty(t, page.Records)
	_, err = os.Stat(missing)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func hooksTestCommand(cwd, host string) (*cobra.Command, *bytes.Buffer) {
	command := &cobra.Command{Use: "megafloyd", SilenceErrors: true, SilenceUsage: true}
	command.PersistentFlags().String("cwd", cwd, "")
	command.PersistentFlags().String("data-dir", "", "")
	command.PersistentFlags().String("host", host, "")
	command.AddCommand(newHooksCommand())
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)
	return command, output
}
