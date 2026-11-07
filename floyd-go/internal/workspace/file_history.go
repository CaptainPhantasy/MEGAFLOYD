package workspace

import (
	"context"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/rewind"
)

// FileRewindWorkspace keeps optional checkpoint support compatible with older
// workspace implementations. History listing remains on the base Workspace.
type FileRewindWorkspace interface {
	RewindSession(context.Context, string, int) (*rewind.Result, error)
}

func (w *AppWorkspace) RewindSession(ctx context.Context, sid string, count int) (result *rewind.Result, err error) {
	apply := func() error {
		result, err = rewind.Rewind(ctx, w.app.Sessions, w.app.Messages, w.app.History, sid, count, w.WorkingDir())
		return err
	}
	if c, ok := w.app.AgentCoordinator.(interface {
		WithSessionIdle(string, func() error) error
	}); ok {
		err = c.WithSessionIdle(sid, apply)
	} else if w.AgentIsSessionBusy(sid) {
		err = agent.ErrSessionBusy
	} else {
		err = apply()
	}
	return
}

func (w *ClientWorkspace) RewindSession(ctx context.Context, sid string, count int) (*rewind.Result, error) {
	return w.client.RewindSession(ctx, w.workspaceID(), sid, count)
}
