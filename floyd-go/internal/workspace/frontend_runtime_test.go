package workspace_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/app"
	"github.com/legacy-ai/floyd/internal/backend"
	"github.com/legacy-ai/floyd/internal/hookstore"
	mem "github.com/legacy-ai/floyd/internal/megafloyd/memory"
	"github.com/legacy-ai/floyd/internal/server"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/client"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/shell"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

type frontendControlCoordinator struct {
	agent.Coordinator
	busy      map[string]bool
	cancelled string
	summaries []string
}

func (c *frontendControlCoordinator) Cancel(id string) { c.cancelled = id }
func (c *frontendControlCoordinator) Summarize(_ context.Context, id string) error {
	c.summaries = append(c.summaries, id)
	return nil
}

func TestWorkspaceFrontendControlsLocalAndRealHTTP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_CONFIG", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_DATA", t.TempDir())
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	h := newFrontendHarness(t)
	cfgPath := filepath.Join(h.workspace.Path, "floyd.json")
	initial := `{"options":{"disable_default_providers":true,"disable_provider_auto_update":true,"auto_lsp":false,"disable_auto_summarize":false},"providers":{"fixture":{"type":"openai-compat","base_url":"http://127.0.0.1:9/v1","api_key":"fixture-key","models":[{"id":"fixture-model","name":"Fixture","context_window":32768,"default_max_tokens":128}]}},"models":{"large":{"provider":"fixture","model":"fixture-model"},"small":{"provider":"fixture","model":"fixture-model"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(initial), 0o600))
	store, err := config.Init(h.workspace.Path, "", false)
	require.NoError(t, err)
	store.SetupAgents()
	h.workspace.Cfg = store
	dataDir := store.Config().Options.DataDirectory
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dataDir)) })
	q := db.New(conn)
	h.app.Sessions = session.NewService(q, conn)
	h.app.Messages = message.NewService(q)
	h.app.History = history.NewService(q, conn)
	coord := &frontendControlCoordinator{busy: map[string]bool{}}
	h.app.AgentCoordinator = coord
	transport, err := client.NewClient(h.workspace.Path, "tcp", strings.TrimPrefix(h.httpSrv.URL, "http://"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(h.workspace.Path, "fixture.go"), []byte("package fixture\n// SEARCH_ME_TRUE\n"), 0o600))
	remote := workspace.NewClientWorkspace(transport, proto.Workspace{ID: h.workspace.ID, Path: h.workspace.Path, Config: store.Config()})
	local := workspace.NewAppWorkspace(h.app, store)
	for _, tc := range []struct {
		name string
		ws   workspace.Workspace
	}{{"local", local}, {"server", remote}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			parent, err := tc.ws.CreateSession(ctx, tc.name)
			require.NoError(t, err)
			other, err := tc.ws.CreateSession(ctx, "other")
			require.NoError(t, err)
			_, err = tc.ws.RalphGoal(ctx, parent.ID, "start", "finish this exact task")
			require.NoError(t, err)
			state, err := tc.ws.RalphGoal(ctx, parent.ID, "status", "")
			require.NoError(t, err)
			require.Equal(t, "finish this exact task", state.Prompt)
			require.Zero(t, state.MaxIterations)
			foreign, err := tc.ws.RalphGoal(ctx, other.ID, "status", "")
			require.NoError(t, err)
			require.Nil(t, foreign)
			_, err = tc.ws.RalphGoal(ctx, other.ID, "cancel", "")
			require.ErrorContains(t, err, "another session")
			_, err = tc.ws.RalphGoal(ctx, parent.ID, "cancel", "")
			require.NoError(t, err)
			require.Equal(t, parent.ID, coord.cancelled)
			state, err = tc.ws.RalphGoal(ctx, parent.ID, "status", "")
			require.NoError(t, err)
			require.Nil(t, state)
			p, err := tc.ws.Plan(ctx, parent.ID, &proto.RuntimePlanRequest{Action: "enter"})
			require.NoError(t, err)
			require.True(t, p.Active)
			_, err = tc.ws.Plan(ctx, parent.ID, &proto.RuntimePlanRequest{Action: "update", Content: "Keep exact acceptance"})
			require.NoError(t, err)
			p, err = tc.ws.Plan(ctx, parent.ID, nil)
			require.NoError(t, err)
			require.Equal(t, "Keep exact acceptance", p.Content)
			p, err = tc.ws.Plan(ctx, parent.ID, &proto.RuntimePlanRequest{Action: "clear"})
			require.NoError(t, err)
			require.True(t, p.Active)
			require.Empty(t, p.Content)
			p, err = tc.ws.Plan(ctx, parent.ID, &proto.RuntimePlanRequest{Action: "cancel"})
			require.NoError(t, err)
			require.False(t, p.Active)
			active, parallel := true, 2
			_, err = tc.ws.SetSwarm(ctx, parent.ID, &active, &parallel)
			require.NoError(t, err)
			remembered, err := remote.Swarm(ctx, parent.ID)
			require.NoError(t, err)
			require.True(t, remembered.Active)
			require.Equal(t, 2, remembered.MaxParallel)
			independent, err := tc.ws.Swarm(ctx, other.ID)
			require.NoError(t, err)
			require.False(t, independent.Active)
			user, err := h.app.Messages.Create(ctx, parent.ID, message.CreateMessageParams{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "First prompt"}}})
			require.NoError(t, err)
			_, err = h.app.Messages.Create(ctx, parent.ID, message.CreateMessageParams{Role: message.Assistant, Parts: []message.ContentPart{message.TextContent{Text: "First result"}}})
			require.NoError(t, err)
			coord.busy[parent.ID] = true
			_, err = tc.ws.ForkSession(ctx, parent.ID)
			require.Error(t, err)
			_, err = tc.ws.UndoPrompts(ctx, parent.ID, 1)
			require.Error(t, err)
			coord.busy[parent.ID] = false
			fork, err := tc.ws.ForkSession(ctx, parent.ID)
			require.NoError(t, err)
			require.Equal(t, 2, fork.CopiedCount)
			require.Equal(t, parent.ID, fork.NewSession.ParentSessionID)
			copied, err := tc.ws.ListMessages(ctx, fork.NewSession.ID)
			require.NoError(t, err)
			require.Len(t, copied, 3)
			require.Equal(t, "First prompt", copied[0].Content().Text)
			require.NotEqual(t, user.ID, copied[0].ID)
			undo, err := tc.ws.UndoPrompts(ctx, parent.ID, 1)
			require.NoError(t, err)
			require.Len(t, undo.RemovedMessageIDs, 2)
			require.NotEmpty(t, undo.RecordID)
			var records int
			require.NoError(t, conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM session_undo_records WHERE id = ?", undo.RecordID).Scan(&records))
			require.Equal(t, 1, records)
			remaining, err := tc.ws.ListMessages(ctx, parent.ID)
			require.NoError(t, err)
			require.Empty(t, remaining)
			require.NoError(t, tc.ws.Summarize(ctx, fork.NewSession.ID, "Preserve acceptance wording"))
			require.Contains(t, coord.summaries, fork.NewSession.ID)
			manager := shell.GetBackgroundShellManager()
			job, err := manager.StartForSession(ctx, h.workspace.Path, nil, "printf task-result", "inspect", parent.ID, h.workspace.Path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = manager.Remove(job.ID) })
			require.True(t, job.WaitContext(ctx))
			task, err := tc.ws.Task(ctx, "bash:"+job.ID, "")
			require.NoError(t, err)
			require.Equal(t, "completed", task.Status)
			require.Equal(t, "task-result", task.OutputPreview)
			op := tc.ws.(workspace.FrontendOperations)
			inspect := func(in proto.FrontendRequest) proto.FrontendResult {
				t.Helper()
				out, err := op.FrontendOperation(ctx, in)
				require.NoError(t, err)
				require.NotEmpty(t, out.Body)
				return out
			}
			doctor := inspect(proto.FrontendRequest{Kind: "doctor"})
			require.Contains(t, doctor.Body, h.workspace.Path)
			require.Contains(t, doctor.Body, "instruction SHA-256")
			analyze := inspect(proto.FrontendRequest{Kind: "codebase", Action: "analyze"})
			require.Contains(t, analyze.Body, "Go")
			search := inspect(proto.FrontendRequest{Kind: "codebase", Action: "search", Argument: "SEARCH_ME_TRUE"})
			require.Contains(t, search.Body, "fixture.go")
			_ = inspect(proto.FrontendRequest{Kind: "codebase", Action: "deps"})
			store.Config().Hooks = map[string][]config.HookConfig{"PreToolUse": {{Command: "touch must-not-execute"}}}
			explain := inspect(proto.FrontendRequest{Kind: "hooks", Action: "explain", Argument: "PreToolUse", ToolName: "view"})
			require.Contains(t, explain.Body, "inspection_only")
			require.NoFileExists(t, filepath.Join(h.workspace.Path, "must-not-execute"))
			require.NoError(t, hookstore.New(conn).Append(ctx, hookstore.Record{ID: "record-" + parent.ID, EventID: "event-" + parent.ID, Event: "PreToolUse", SessionID: parent.ID, State: "completed", Outcome: "success"}))
			trace := inspect(proto.FrontendRequest{Kind: "hooks", Action: "trace", SessionID: parent.ID})
			require.Contains(t, trace.Body, "record-"+parent.ID)
			require.NoError(t, (mem.Writer{CWD: h.workspace.Path, DataDir: dataDir}).Write(ctx, mem.Checkpoint{SessionID: parent.ID, Note: "Remember acceptance"}))
			checkpoint := inspect(proto.FrontendRequest{Kind: "memory"})
			require.Contains(t, checkpoint.Body, "Remember acceptance")
			require.Equal(t, filepath.Join(dataDir, mem.FileName), checkpoint.Path)
			exported := inspect(proto.FrontendRequest{Kind: "session", Action: "export", SessionID: fork.NewSession.ID})
			require.DirExists(t, exported.Path)
			require.NotEmpty(t, exported.SessionID)
			imported := inspect(proto.FrontendRequest{Kind: "session", Action: "import", Argument: exported.SessionID})
			restored, err := tc.ws.ListMessages(ctx, imported.SessionID)
			require.NoError(t, err)
			require.Len(t, restored, 3)
			require.Equal(t, "First prompt", restored[0].Content().Text)
			collected := inspect(proto.FrontendRequest{Kind: "telemetry", Action: "collect"})
			require.FileExists(t, collected.Path)
			report := inspect(proto.FrontendRequest{Kind: "telemetry", Action: "report"})
			require.Contains(t, report.Body, "sessions")
			shipped := make(chan bool, 1)
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				err := json.NewDecoder(r.Body).Decode(&payload)
				shipped <- err == nil && payload["resourceSpans"] != nil
				w.WriteHeader(http.StatusOK)
			}))
			defer sink.Close()
			_ = inspect(proto.FrontendRequest{Kind: "telemetry", Action: "ship", Argument: sink.URL})
			require.True(t, <-shipped)
			_, err = tc.ws.Task(ctx, "bash:missing", "cancel")
			require.ErrorContains(t, err, "not found")
		})
	}
	require.NoError(t, os.WriteFile(cfgPath, []byte(strings.Replace(initial, `"disable_auto_summarize":false`, `"disable_auto_summarize":true`, 1)), 0o600))
	require.NoError(t, remote.ReloadConfig(t.Context()))
	require.True(t, remote.Config().Options.DisableAutoSummarize)
	require.True(t, store.Config().Options.DisableAutoSummarize)
	// A malformed edit surfaces a real error and preserves last loaded settings.
	require.NoError(t, os.WriteFile(cfgPath, []byte("{"), 0o600))
	require.Error(t, remote.ReloadConfig(t.Context()))
	require.True(t, store.Config().Options.DisableAutoSummarize)
}

func (c *frontendControlCoordinator) IsSessionBusy(id string) bool       { return c.busy[id] }
func (c *frontendControlCoordinator) IsBusy() bool                       { return false }
func (c *frontendControlCoordinator) UpdateModels(context.Context) error { return nil }

type frontendHarness struct {
	app       *app.App
	workspace *backend.Workspace
	httpSrv   *httptest.Server
}

func newFrontendHarness(t *testing.T) *frontendHarness {
	ctx, cancel := context.WithCancel(t.Context())
	a := app.NewForTest(ctx)
	t.Cleanup(func() { cancel(); a.ShutdownForTest() })
	srv := server.NewServer(nil, "tcp", "127.0.0.1:0")
	srv.SetLogger(slog.Default())
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	ws := &backend.Workspace{ID: "frontend-fixture", Path: t.TempDir(), App: a}
	backend.SetWorkspaceShutdownFnForTest(ws, func() {})
	backend.InsertWorkspaceForTest(srv.Backend(), ws)
	return &frontendHarness{app: a, workspace: ws, httpSrv: hs}
}
func (c *frontendControlCoordinator) QueuedPrompts(string) int { return 0 }
