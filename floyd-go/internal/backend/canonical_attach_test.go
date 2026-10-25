package backend

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/stretchr/testify/require"
)

type canonicalAttachedCoordinator struct {
	*blockingCoordinator
}

func (c *canonicalAttachedCoordinator) IsBusy() bool {
	select {
	case <-c.release:
		return false
	default:
		return c.runCount.Load() > 0
	}
}

func TestCanonicalAttachPreservesActiveCoordinator(t *testing.T) {
	b, _ := newTestBackend(t)
	coord := &canonicalAttachedCoordinator{newBlockingCoordinator()}
	ws := insertAgentWorkspace(t, b, coord)
	var release sync.Once
	t.Cleanup(func() {
		release.Do(func() { close(coord.release) })
		ws.runWG.Wait()
	})
	require.NoError(t, b.SendMessage(ws.ID, proto.AgentMessage{SessionID: "same-session", Prompt: "continue"}))
	select {
	case <-coord.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("active run did not start")
	}

	// A TUI and a web client may attach concurrently while this run waits.
	errs := make(chan error, 2)
	for _, interactive := range []bool{false, true} {
		go func() { errs <- b.InitAgent(context.Background(), ws.ID, interactive) }()
	}
	for range 2 {
		require.NoError(t, <-errs)
	}
	require.Same(t, coord, ws.AgentCoordinator)
	info, err := b.GetAgentInfo(ws.ID)
	require.NoError(t, err)
	require.True(t, info.IsReady)
	require.True(t, info.IsBusy, "reattaching must preserve the in-flight run")
	require.Equal(t, int32(1), coord.runCount.Load())

	release.Do(func() { close(coord.release) })
	ws.runWG.Wait()
	info, err = b.GetAgentInfo(ws.ID)
	require.NoError(t, err)
	require.False(t, info.IsBusy)

	ws.runMu.Lock()
	ws.closing = true
	ws.runMu.Unlock()
	require.ErrorIs(t, b.InitAgent(context.Background(), ws.ID, true), ErrWorkspaceClosing)
}
