package rewind

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/message"
	"github.com/legacy-ai/floyd/internal/session"
)

var sessionRewinds sync.Map

type Result struct {
	Transcript    *session.UndoResult `json:"transcript"`
	RestoredFiles []string            `json:"restored_files"`
	RecoveryPath  string              `json:"recovery_path"`
}

// Rewind restores native file checkpoints and removes the corresponding turns
// from the active transcript, retaining both in recovery storage. The caller
// holds the agent's session-idle guard throughout this operation.
func Rewind(ctx context.Context, sessions session.Service, messages message.Service, files history.Service, sid string, count int, root string) (*Result, error) {
	lock, _ := sessionRewinds.LoadOrStore(root+"\x00"+sid, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	undo, ok := sessions.(interface {
		Undo(context.Context, string, int) (*session.UndoResult, error)
	})
	if !ok {
		return nil, errors.New("session undo is unavailable")
	}
	restore, ok := files.(history.TurnRestorer)
	if !ok {
		return nil, errors.New("file checkpoint restoration is unavailable")
	}
	publisher, ok := messages.(interface{ NotifyDeleted([]message.Message) })
	if !ok {
		return nil, errors.New("message service cannot publish removed transcript rows")
	}
	if err := messages.FlushAll(ctx); err != nil {
		return nil, err
	}
	before, err := message.PublicList(ctx, messages, sid)
	if err != nil {
		return nil, err
	}
	var users []string
	for _, msg := range before {
		if msg.Role == message.User {
			users = append(users, msg.ID)
		}
	}
	if count < 1 || count > len(users) {
		return nil, session.ErrInvalidUndo
	}
	selected := false
	for _, msg := range before {
		if msg.ID == users[len(users)-count] {
			selected = true
		}
		if !selected {
			continue
		}
		for _, call := range msg.ToolCalls() {
			switch call.Name {
			case "write", "edit", "multiedit", "lsp_rename", "lsp_replace_symbol", "view", "read", "ls", "glob", "grep", "lsp_diagnostics", "lsp_references", "lsp_document_symbols", "lsp_hover", "question", "todos", "todo":
			default:
				return nil, fmt.Errorf("rewind cannot verify file effects of tool %q in the selected turns; use /undo for transcript-only recovery", call.Name)
			}
		}
	}
	result := &Result{}
	result.RestoredFiles, result.RecoveryPath, err = restore.RestoreTurns(ctx, sid, users[len(users)-count:], root, func() error {
		var err error
		result.Transcript, err = undo.Undo(ctx, sid, count)
		return err
	})
	if err != nil {
		return nil, err
	}
	removed := make(map[string]bool, len(result.Transcript.RemovedMessageIDs))
	for _, id := range result.Transcript.RemovedMessageIDs {
		removed[id] = true
	}
	var deleted []message.Message
	for _, msg := range before {
		if removed[msg.ID] {
			deleted = append(deleted, msg)
		}
	}
	publisher.NotifyDeleted(deleted)
	return result, nil
}
