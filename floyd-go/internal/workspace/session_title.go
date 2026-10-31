package workspace

import "context"

// SessionTitleWorkspace updates only the title. It cannot overwrite usage,
// messages, or metadata with a stale UI copy of the session.
type SessionTitleWorkspace interface {
	RenameSession(context.Context, string, string) error
}

func (w *AppWorkspace) RenameSession(ctx context.Context, sid, title string) error {
	return w.app.Sessions.Rename(ctx, sid, title)
}

func (w *ClientWorkspace) RenameSession(ctx context.Context, sid, title string) error {
	return w.client.RenameSession(ctx, w.workspaceID(), sid, title)
}
