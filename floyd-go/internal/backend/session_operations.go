package backend

import (
	"context"
	"fmt"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
)

type sessionOperations interface {
	SetArchived(context.Context, string, bool) (session.Session, error)
	UpdateMetadata(context.Context, string, *string, map[string]any) (session.Session, error)
	Clone(context.Context, string, *string, map[string]any, bool) (session.Session, error)
	Undo(context.Context, string, int) (*session.UndoResult, error)
}

func (b *Backend) idleSessionOperation(workspaceID, sessionID string, operation func(*Workspace) error) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}
	ws.runMu.Lock()
	defer ws.runMu.Unlock()
	if ws.closing {
		return ErrWorkspaceClosing
	}
	if ws.AgentCoordinator != nil {
		if guard, ok := ws.AgentCoordinator.(interface {
			WithSessionIdle(string, func() error) error
		}); ok {
			return guard.WithSessionIdle(sessionID, func() error { return operation(ws) })
		}
		if ws.AgentCoordinator.IsSessionBusy(sessionID) || ws.AgentCoordinator.QueuedPrompts(sessionID) > 0 {
			return agent.ErrSessionBusy
		}
	}
	return operation(ws)
}

func sessionManager(ws *Workspace) (sessionOperations, error) {
	service, ok := ws.Sessions.(sessionOperations)
	if !ok {
		return nil, fmt.Errorf("workspace session service cannot perform durable session operations")
	}
	return service, nil
}

func (b *Backend) SetSessionArchived(ctx context.Context, workspaceID, sessionID string, archived bool) (result session.Session, err error) {
	err = b.idleSessionOperation(workspaceID, sessionID, func(ws *Workspace) error {
		service, e := sessionManager(ws)
		if e != nil {
			return e
		}
		result, e = service.SetArchived(ctx, sessionID, archived)
		return e
	})
	return
}

func (b *Backend) UpdateSessionMetadata(ctx context.Context, workspaceID, sessionID string, request proto.SessionMetadataRequest) (session.Session, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return session.Session{}, err
	}
	service, err := sessionManager(ws)
	if err != nil {
		return session.Session{}, err
	}
	return service.UpdateMetadata(ctx, sessionID, request.Title, request.Metadata)
}

func (b *Backend) CloneSession(ctx context.Context, workspaceID, sessionID string, request proto.SessionCloneRequest, branch bool) (result session.Session, err error) {
	err = b.idleSessionOperation(workspaceID, sessionID, func(ws *Workspace) error {
		if e := ws.Messages.FlushAll(ctx); e != nil {
			return e
		}
		service, e := sessionManager(ws)
		if e != nil {
			return e
		}
		result, e = service.Clone(ctx, sessionID, request.Title, request.Metadata, branch)
		return e
	})
	return
}

func (b *Backend) UndoSession(ctx context.Context, workspaceID, sessionID string, count int) (result *session.UndoResult, err error) {
	err = b.idleSessionOperation(workspaceID, sessionID, func(ws *Workspace) error {
		if e := ws.Messages.FlushAll(ctx); e != nil {
			return e
		}
		before, e := ws.Messages.List(ctx, sessionID)
		if e != nil {
			return e
		}
		service, e := sessionManager(ws)
		if e != nil {
			return e
		}
		publisher, ok := ws.Messages.(interface{ NotifyDeleted([]message.Message) })
		if !ok {
			return fmt.Errorf("message service cannot publish deleted rows")
		}
		result, e = service.Undo(ctx, sessionID, count)
		if e != nil {
			return e
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
	})
	return
}

func (b *Backend) sessionHistory(ctx context.Context, workspaceID, sessionID string) (*Workspace, history.TurnRecorder, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := ws.Sessions.Get(ctx, sessionID); err != nil {
		return nil, nil, err
	}
	recorder, ok := ws.History.(history.TurnRecorder)
	if !ok {
		return nil, nil, fmt.Errorf("workspace history service cannot read turn checkpoints")
	}
	return ws, recorder, nil
}

func (b *Backend) ListSessionTurns(ctx context.Context, workspaceID, sessionID string) ([]history.Turn, error) {
	_, recorder, err := b.sessionHistory(ctx, workspaceID, sessionID)
	if err != nil {
		return nil, err
	}
	return recorder.ListTurns(ctx, sessionID)
}

func (b *Backend) SessionTurnChanges(ctx context.Context, workspaceID, sessionID string, ordinal int64) (history.TurnChanges, error) {
	ws, recorder, err := b.sessionHistory(ctx, workspaceID, sessionID)
	if err != nil {
		return history.TurnChanges{}, err
	}
	return recorder.TurnChanges(ctx, sessionID, ordinal, ws.Path)
}

func (b *Backend) SessionTurnContent(ctx context.Context, workspaceID, sessionID string, ordinal int64, path, phase string) (*history.TurnContent, error) {
	ws, recorder, err := b.sessionHistory(ctx, workspaceID, sessionID)
	if err != nil {
		return nil, err
	}
	return recorder.TurnContent(ctx, sessionID, ordinal, path, phase, ws.Path)
}
