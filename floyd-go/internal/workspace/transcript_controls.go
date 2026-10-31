package workspace

import (
	"context"
	"errors"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

func (w *AppWorkspace) UndoPrompts(ctx context.Context, sid string, count int) (result *session.UndoResult, err error) {
	apply := func() error {
		if err := w.app.Messages.FlushAll(ctx); err != nil {
			return err
		}
		service, ok := w.app.Sessions.(interface {
			Undo(context.Context, string, int) (*session.UndoResult, error)
		})
		if !ok {
			return errors.New("session undo is unavailable")
		}
		publisher, ok := w.app.Messages.(interface{ NotifyDeleted([]message.Message) })
		if !ok {
			return errors.New("message service cannot publish removed transcript rows")
		}
		before, err := message.PublicList(ctx, w.app.Messages, sid)
		if err != nil {
			return err
		}
		result, err = service.Undo(ctx, sid, count)
		if err != nil {
			return err
		}
		removed := make(map[string]bool, len(result.RemovedMessageIDs))
		for _, id := range result.RemovedMessageIDs {
			removed[id] = true
		}
		var deleted []message.Message
		for _, msg := range before {
			if removed[msg.ID] {
				deleted = append(deleted, msg)
			}
		}
		publisher.NotifyDeleted(deleted)
		return nil
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

func (w *ClientWorkspace) UndoPrompts(ctx context.Context, sid string, count int) (*session.UndoResult, error) {
	result, err := w.client.UndoPrompts(ctx, w.workspaceID(), sid, count)
	if err != nil {
		return nil, err
	}
	return &session.UndoResult{Session: protoToSession(result.Session), RemovedTurns: result.RemovedTurns, RemovedMessageIDs: result.RemovedMessageIDs, RecordID: result.UndoRecordID}, nil
}

func (w *AppWorkspace) Summarize(ctx context.Context, sid, instruction string) error {
	return w.AgentSummarize(agent.WithSummaryInstruction(ctx, instruction), sid)
}

func (w *ClientWorkspace) Summarize(ctx context.Context, sid, instruction string) error {
	return w.client.CompactSession(ctx, w.workspaceID(), sid, instruction)
}
