package model

import (
	"context"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
	"github.com/legacy-ai/floyd/internal/ui/common"
	"github.com/legacy-ai/floyd/internal/ui/dialog"
	"github.com/legacy-ai/floyd/internal/ui/styles"
	"github.com/legacy-ai/floyd/internal/ui/util"
	"github.com/legacy-ai/floyd/internal/workspace"
	"github.com/stretchr/testify/require"
)

// swarmTestWorkspace records swarm state changes so the command can be
// tested without a real session service.
type swarmTestWorkspace struct {
	workspace.Workspace
	cfg      *config.Config
	state    proto.SwarmState
	setCalls int
}

func (w *swarmTestWorkspace) Config() *config.Config { return w.cfg }

func (w *swarmTestWorkspace) Swarm(context.Context, string) (proto.SwarmState, error) {
	return w.state, nil
}

func (w *swarmTestWorkspace) SetSwarm(_ context.Context, _ string, active *bool, parallel *int) (proto.SwarmState, error) {
	if active != nil {
		w.state.Active = *active
	}
	if parallel != nil {
		w.state.MaxParallel = *parallel
	}
	w.setCalls++
	return w.state, nil
}

func newSwarmTestUI(ws *swarmTestWorkspace) *UI {
	st := styles.CharmtonePantera()
	com := &common.Common{Workspace: ws, Styles: &st}
	return &UI{
		com:     com,
		dialog:  dialog.NewOverlay(),
		status:  NewStatus(com, nil),
		session: &session.Session{ID: "sess-1", Title: "Work"},
	}
}

func TestSwarmOnEnablesParallelDispatch(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	cmd := ui.runBuiltin("swarm", "on", false)
	require.NotNil(t, cmd, "/swarm on must run")

	msg := cmd()
	status, ok := msg.(swarmStatusMsg)
	require.True(t, ok, "/swarm on must report swarm state, got %T", msg)
	require.NoError(t, status.Err)
	require.True(t, ws.state.Active, "/swarm on must enable swarm mode")
	require.Equal(t, 1, ws.setCalls)
	require.Contains(t, status.Report, "Swarm mode on")
}

func TestSwarmOffDisablesParallelDispatch(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{Active: true, MaxParallel: 4}}
	ui := newSwarmTestUI(ws)

	msg := ui.runBuiltin("swarm", "off", false)()
	status, ok := msg.(swarmStatusMsg)
	require.True(t, ok, "/swarm off must report swarm state, got %T", msg)
	require.False(t, ws.state.Active, "/swarm off must disable swarm mode")
	require.Equal(t, 4, ws.state.MaxParallel, "turning swarm off must keep the parallel ceiling")
	require.Contains(t, status.Report, "Swarm mode off")
}

func TestSwarmParallelValidatesRange(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{Active: true, MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	msg := ui.runBuiltin("swarm", "parallel 6", false)()
	_, ok := msg.(swarmStatusMsg)
	require.True(t, ok, "/swarm parallel 6 must report swarm state, got %T", msg)
	require.Equal(t, 6, ws.state.MaxParallel)

	for _, bad := range []string{"parallel 0", "parallel 9", "parallel", "parallel many"} {
		cmd := ui.runBuiltin("swarm", bad, false)
		require.NotNil(t, cmd)
		im, isInfo := cmd().(util.InfoMsg)
		require.True(t, isInfo, "%q must warn, got %T", bad, cmd())
		require.Equal(t, util.InfoTypeWarn, im.Type, "%q must warn", bad)
		require.Contains(t, im.Msg, "parallel")
	}
	require.Equal(t, 1, ws.setCalls, "rejected parallelism must not be written")
}

func TestSwarmOnIsNotDispatchedAsTask(t *testing.T) {
	t.Parallel()

	// The regression: /swarm on used to treat "on" as a task prompt and
	// fan it out to every configured agent.
	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	msg := ui.runBuiltin("swarm", "on", false)()
	status, ok := msg.(swarmStatusMsg)
	require.True(t, ok, "/swarm on must not dispatch a task, got %T", msg)
	require.Empty(t, status.Body)
	require.True(t, ws.state.Active)
}

func TestSwarmUnknownArgumentShowsUsage(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	im, ok := ui.runBuiltin("swarm", "please do the thing", false)().(util.InfoMsg)
	require.True(t, ok)
	require.Equal(t, util.InfoTypeWarn, im.Type)
	require.Contains(t, im.Msg, "Usage: /swarm")
	require.Equal(t, 0, ws.setCalls, "an unknown argument must not change state")
}

func TestSwarmRunKeepsFanOutPath(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	// No agents are configured in the test config, so the fan-out path
	// reports that instead of falling through to the usage warning.
	im, ok := ui.runBuiltin("swarm", "run summarise the repo", false)().(util.InfoMsg)
	require.True(t, ok, "expected a status message")
	require.Contains(t, im.Msg, "No agents are configured")
	require.Equal(t, 0, ws.setCalls, "fan-out must not touch swarm mode")
}

func TestSwarmStatusShowsStateAndPill(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{Active: true, MaxParallel: 5}}
	ui := newSwarmTestUI(ws)

	msg := ui.runBuiltin("swarm", "status", false)()
	status, ok := msg.(swarmStatusMsg)
	require.True(t, ok)
	require.Contains(t, status.Body, "Swarm mode: on")
	require.Contains(t, status.Body, "Subagent parallelism: 5")

	_, _ = ui.Update(status)
	require.True(t, ui.status.swarmActive, "an active swarm must show the pill")
}

func TestTypedSwarmOnReachesHandler(t *testing.T) {
	t.Parallel()

	ws := &swarmTestWorkspace{cfg: testConfigWithModel(1000), state: proto.SwarmState{MaxParallel: 3}}
	ui := newSwarmTestUI(ws)

	cmd, handled := ui.dispatchSlashInput("/swarm on")
	require.True(t, handled, "a typed /swarm must be handled, not sent as a prompt")
	msg := cmd()
	_, ok := msg.(swarmStatusMsg)
	require.True(t, ok, "typed /swarm on must update swarm state, got %T", msg)
	require.True(t, ws.state.Active)
}
