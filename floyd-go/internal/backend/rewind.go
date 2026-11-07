package backend

import (
	"context"

	"github.com/legacy-ai/floyd/internal/rewind"
)

func (b *Backend) RewindSession(ctx context.Context, wid, sid string, count int) (result *rewind.Result, err error) {
	err = b.idleSessionOperation(wid, sid, func(ws *Workspace) error {
		result, err = rewind.Rewind(ctx, ws.Sessions, ws.Messages, ws.History, sid, count, ws.Path)
		return err
	})
	return
}
