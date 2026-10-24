package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/csync"
	"github.com/stretchr/testify/require"
)

type runtimeBlockedChild struct {
	SessionAgent
	entered chan struct{}
	running atomic.Bool
}

func (a *runtimeBlockedChild) CancelAll()                {}
func (a *runtimeBlockedChild) Cancel(string)             {}
func (a *runtimeBlockedChild) IsBusy() bool              { return a.running.Load() }
func (a *runtimeBlockedChild) IsSessionBusy(string) bool { return a.running.Load() }
func (a *runtimeBlockedChild) Model() Model {
	return Model{ModelCfg: config.SelectedModel{Provider: "unit-only"}}
}
func (a *runtimeBlockedChild) Run(ctx context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
	a.running.Store(true)
	defer a.running.Store(false)
	close(a.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCanonicalSubagentDetachAndCancelUseSameChild(t *testing.T) {
	env := testEnv(t)
	parent, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)
	providers := csync.NewMap[string, config.ProviderConfig]()
	providers.Set("unit-only", config.ProviderConfig{})
	c := &coordinator{sessions: env.sessions, cfg: config.NewTestStore(&config.Config{Providers: providers})}
	t.Cleanup(func() { c.RuntimeCancelTasks(); runtimeTaskStores.Delete(c) })
	child := &runtimeBlockedChild{entered: make(chan struct{})}
	parentCtx, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()
	result := make(chan error, 1)
	go func() {
		_, err := c.runSubAgent(parentCtx, subAgentParams{Agent: child, SessionID: parent.ID, AgentMessageID: "message", ToolCallID: "tool", Prompt: "controlled task", SessionTitle: "Child"})
		result <- err
	}()
	select {
	case <-child.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("existing child engine was not called")
	}
	tasks := c.RuntimeTasks()
	require.Len(t, tasks, 1)
	id := tasks[0].ID
	require.Equal(t, parent.ID, tasks[0].SessionID)
	selected, ok := c.RuntimeChildAgent(id)
	require.True(t, ok)
	require.Same(t, child, selected)
	require.NoError(t, c.RuntimeTaskControl(id, "detach"))
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("detach did not release parent tool call")
	}
	cancelParent()
	require.Eventually(t, func() bool { tasks = c.RuntimeTasks(); return tasks[0].RunInBackground }, time.Second, time.Millisecond)
	require.Equal(t, "running", tasks[0].Status, "detached child must outlive the parent's turn")
	require.NoError(t, c.RuntimeTaskControl(id, "cancel"))
	require.Eventually(t, func() bool { return c.RuntimeTasks()[0].Status == "cancelled" }, time.Second, time.Millisecond)
	stored, err := env.sessions.Get(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, parent.ID, stored.ParentSessionID)
}

func TestCanonicalSwarmBoundsAndProfilePersistence(t *testing.T) {
	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	t.Setenv("MEGAFLOYD_HOME", t.TempDir())
	env := testEnv(t)
	parent, err := env.sessions.Create(t.Context(), "Swarm")
	require.NoError(t, err)
	c := &coordinator{sessions: env.sessions, cfg: config.NewTestStoreWithWorkingDir(&config.Config{}, t.TempDir())}
	t.Cleanup(func() { runtimeTaskStores.Delete(c) })
	active, parallel := true, 2
	_, err = SetRuntimeSwarm(t.Context(), env.sessions, parent.ID, &active, &parallel)
	require.NoError(t, err)
	release1, err := c.acquireRuntimeTask(t.Context(), parent.ID)
	require.NoError(t, err)
	release2, err := c.acquireRuntimeTask(t.Context(), parent.ID)
	require.NoError(t, err)
	defer release2()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	third := make(chan func(), 1)
	go func() {
		release, err := c.acquireRuntimeTask(ctx, parent.ID)
		if err == nil {
			third <- release
		}
	}()
	select {
	case release := <-third:
		release()
		t.Fatal("swarm exceeded max_parallel")
	case <-time.After(20 * time.Millisecond):
	}
	release1()
	select {
	case release := <-third:
		release()
	case <-time.After(time.Second):
		t.Fatal("waiting child did not acquire released slot")
	}
	_, err = c.RuntimeSaveProfile("reviewer", RuntimeProfile{Description: "Review only", SystemPrompt: "Report concrete defects", Model: config.SelectedModelTypeSmall, AllowedTools: []string{}})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(c.cfg.WorkingDir(), ".megafloyd", "agents", "reviewer.md"))
	require.NoError(t, err)
	profiles, err := c.RuntimeProfiles()
	require.NoError(t, err)
	var found *RuntimeProfile
	for i := range profiles {
		if profiles[i].ID == "reviewer" {
			found = &profiles[i]
		}
	}
	require.NotNil(t, found)
	require.Equal(t, "Report concrete defects", found.SystemPrompt)
	require.NotNil(t, found.AllowedTools, "explicit empty allowlist must not become inherited tools")
	require.Empty(t, found.AllowedTools)
	_, err = c.RuntimeSaveProfile("../escape", *found)
	require.Error(t, err)
}

func TestCanonicalPluginReloadWaitsForAcceptedRuns(t *testing.T) {
	env := testEnv(t)
	a := NewSessionAgent(SessionAgentOptions{Sessions: env.sessions, Messages: env.messages}).(*sessionAgent)
	c := &coordinator{currentAgent: a}
	t.Cleanup(func() { a.CancelAll(); runtimeTaskStores.Delete(c) })
	accepted := a.BeginAccepted("accepted-before-start")
	changed := false
	require.ErrorIs(t, c.RuntimeWithIdle(func() error { changed = true; return nil }), ErrSessionBusy)
	require.False(t, changed, "reload must not overtake an accepted prompt")
	accepted.Close()
	require.NoError(t, c.RuntimeWithIdle(func() error { changed = true; return nil }))
	require.True(t, changed)
}
