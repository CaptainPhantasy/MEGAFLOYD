package workspace_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/client"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/filetracker"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestNamedSwarmRealHTTPCompletionAndCancellation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_CONFIG", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_DATA", t.TempDir())
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	type modelRequest struct {
		Model string `json:"model"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		Stream bool `json:"stream"`
	}
	requests := make(chan modelRequest, 8)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in modelRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if !in.Stream {
			fmt.Fprintf(w, `{"id":"title","choices":[{"index":0,"message":{"role":"assistant","content":"Fixture title"},"finish_reason":"stop"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Server child progress. \"}}]}\n\n", in.Model)
		w.(http.Flusher).Flush()
		if len(in.Tools) > 0 {
			requests <- in
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"model\":%q,\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Completed child work.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", in.Model)
	}))
	t.Cleanup(model.Close)
	t.Cleanup(unblock)
	h := newFrontendHarness(t)
	cfgText := fmt.Sprintf(`{"options":{"disable_default_providers":true,"disable_provider_auto_update":true,"auto_lsp":false},"providers":{"fixture":{"type":"openai-compat","base_url":%q,"api_key":"fixture-key","models":[{"id":"writer","name":"Writer","context_window":32768,"default_max_tokens":128},{"id":"reader","name":"Reader","context_window":32768,"default_max_tokens":128}]}},"models":{"large":{"provider":"fixture","model":"writer"},"small":{"provider":"fixture","model":"reader"}},"agents":{"task":{"model":"small"}}}`, model.URL+"/v1")
	require.NoError(t, os.WriteFile(filepath.Join(h.workspace.Path, "floyd.json"), []byte(cfgText), 0o600))
	cfg, err := config.Init(h.workspace.Path, "", false)
	require.NoError(t, err)
	cfg.SetupAgents()
	h.workspace.Cfg = cfg
	dir := cfg.Config().Options.DataDirectory
	conn, err := db.Connect(t.Context(), dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Release(dir)) })
	q := db.New(conn)
	h.app.Sessions = session.NewService(q, conn)
	h.app.Messages = message.NewService(q)
	h.app.History = history.NewService(q, conn)
	h.app.FileTracker = filetracker.NewService(q)
	c, err := agent.NewCoordinator(t.Context(), agent.CoordinatorOptions{Config: cfg, Sessions: h.app.Sessions, Messages: h.app.Messages, History: h.app.History, FileTracker: h.app.FileTracker, Permissions: h.app.Permissions, Questions: h.app.Questions})
	require.NoError(t, err)
	h.app.AgentCoordinator = c
	t.Cleanup(c.CancelAll)
	cl, err := client.NewClient(h.workspace.Path, "tcp", strings.TrimPrefix(h.httpSrv.URL, "http://"))
	require.NoError(t, err)
	snapshot, err := cl.GetWorkspace(t.Context(), h.workspace.ID)
	require.NoError(t, err)
	ws := workspace.NewClientWorkspace(cl, *snapshot)
	parent, err := ws.CreateSession(t.Context(), "server swarm")
	require.NoError(t, err)
	_, err = ws.Plan(t.Context(), parent.ID, &proto.RuntimePlanRequest{Action: "enter"})
	require.NoError(t, err)
	active, parallel := false, 2
	_, err = ws.SetSwarm(t.Context(), parent.ID, &active, &parallel)
	require.NoError(t, err)
	writer, err := ws.StartAgentTask(permission.WithNonInteractive(t.Context()), parent.ID, "Inspect only", config.AgentCoder)
	require.NoError(t, err)
	task, err := ws.StartAgentTask(permission.WithNonInteractive(t.Context()), parent.ID, "Inspect only", config.AgentTask)
	require.NoError(t, err)
	require.NotEqual(t, writer.ID, task.ID)
	require.Equal(t, "writer", writer.Model)
	require.Equal(t, "reader", task.Model)
	seen := []string{}
	select {
	case selected := <-requests:
		seen = append(seen, selected.Model)
	case <-time.After(5 * time.Second):
		t.Fatal("first child did not start")
	}
	waiting, err := ws.Task(t.Context(), task.ID, "")
	require.NoError(t, err)
	require.Equal(t, "queued", waiting.Status)
	active = true
	_, err = ws.SetSwarm(t.Context(), parent.ID, &active, nil)
	require.NoError(t, err)
	for range 1 {
		select {
		case model := <-requests:
			seen = append(seen, model.Model)
		case <-time.After(5 * time.Second):
			t.Fatal("server did not dispatch independent agents concurrently")
		}
	}
	require.ElementsMatch(t, []string{"writer", "reader"}, seen)
	require.Eventually(t, func() bool {
		status, err := ws.Task(t.Context(), writer.ID, "")
		return err == nil && status.Status == "running" && strings.Contains(status.OutputPreview, "Server child progress.")
	}, time.Second, 10*time.Millisecond)
	_, err = ws.Task(t.Context(), task.ID, "cancel")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		status, err := ws.Task(t.Context(), task.ID, "")
		return err == nil && status.Status == "cancelled"
	}, time.Second, 10*time.Millisecond)
	unblock()
	require.Eventually(t, func() bool {
		status, err := ws.Task(t.Context(), writer.ID, "")
		return err == nil && status.Status == "completed" && strings.Contains(status.OutputPreview, "Completed child work.")
	}, 3*time.Second, 10*time.Millisecond)
	for _, id := range []string{writer.ID, task.ID} {
		child, err := ws.GetSession(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, parent.ID, child.ParentSessionID)
		inherited, err := ws.Plan(t.Context(), child.ID, nil)
		require.NoError(t, err)
		require.True(t, inherited.Active, "children must inherit the parent's plan restriction")
	}
	// Reconstruct the coordinator from the same durable database, exactly as a
	// server restart does. No in-memory task identity is carried forward.
	require.Eventually(t, func() bool { return !c.IsBusy() }, time.Second, time.Millisecond)
	c.CancelAll()
	restarted, err := agent.NewCoordinator(t.Context(), agent.CoordinatorOptions{Config: cfg, Sessions: h.app.Sessions, Messages: h.app.Messages, History: h.app.History, FileTracker: h.app.FileTracker, Permissions: h.app.Permissions, Questions: h.app.Questions})
	require.NoError(t, err)
	h.app.AgentCoordinator = restarted
	t.Cleanup(restarted.CancelAll)
	require.NoError(t, ws.AgentRun(t.Context(), task.ID, "Continue the same read-only work"))
	select {
	case resumed := <-requests:
		require.Equal(t, "reader", resumed.Model)
		names := make([]string, 0, len(resumed.Tools))
		for _, tool := range resumed.Tools {
			names = append(names, tool.Function.Name)
		}
		require.Contains(t, names, "view")
		for _, name := range []string{"write", "edit", "bash", "parallel_bash", "apply_patch"} {
			require.NotContains(t, names, name, "restart must preserve read-only tools")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("saved child did not resume after coordinator reconstruction")
	}
	require.Eventually(t, func() bool {
		status, err := ws.Task(t.Context(), task.ID, "")
		return err == nil && status.Status == "completed" && status.SubagentType == config.AgentTask && status.Model == "reader" && strings.Contains(status.OutputPreview, "Completed child work.")
	}, 3*time.Second, 10*time.Millisecond)
	// Cancellation between acceptance and dispatch must also reach the task
	// status, without making another provider call.
	accepted := restarted.BeginAccepted(task.ID)
	restarted.Cancel(task.ID)
	_, err = restarted.RunAccepted(t.Context(), accepted, task.ID, "Cancelled before dispatch")
	accepted.Close()
	require.NoError(t, err)
	cancelled, err := ws.Task(t.Context(), task.ID, "")
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	select {
	case <-requests:
		t.Fatal("cancelled accepted child invoked the provider")
	default:
	}
	// A fork has an ordinary UUID but retains the saved agent definition.
	// It must not gain coder tools when resumed through the same HTTP path.
	fork, err := ws.ForkSession(t.Context(), task.ID)
	require.NoError(t, err)
	require.NoError(t, ws.AgentRun(t.Context(), fork.NewSession.ID, "Continue in the fork"))
	select {
	case resumed := <-requests:
		require.Equal(t, "reader", resumed.Model)
		for _, tool := range resumed.Tools {
			require.NotContains(t, []string{"write", "edit", "bash", "parallel_bash", "apply_patch"}, tool.Function.Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forked child did not resume with its saved identity")
	}
	require.Eventually(t, func() bool { return !restarted.IsBusy() }, 3*time.Second, 10*time.Millisecond)
}
