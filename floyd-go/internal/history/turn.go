package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/diff"
)

const maxTurnFileBytes = 4 * 1024 * 1024

type Turn struct {
	TurnID        int64  `json:"turn_id"`
	UserMessageID string `json:"user_message_id"`
	Recorded      bool   `json:"recorded"`
}

type TurnChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	Oversize  bool   `json:"oversize,omitempty"`
}

type TurnChanges struct {
	Changes  []TurnChange `json:"changes"`
	Recorded bool         `json:"recorded"`
}

type TurnContent struct {
	Version int64   `json:"version"`
	Content *string `json:"content,omitempty"`
	Binary  bool    `json:"binary,omitempty"`
}

// TurnRecorder records native file edits against the actual user message.
// Implementations retain data in the same canonical database as the session.
type TurnRecorder interface {
	BeginTurn(context.Context, string, string, string) (context.Context, error)
	CompleteTurn(context.Context) error
	CaptureTurnPath(context.Context, string) error
	ListTurns(context.Context, string) ([]Turn, error)
	TurnChanges(context.Context, string, int64, string) (TurnChanges, error)
	TurnContent(context.Context, string, int64, string, string, string) (*TurnContent, error)
}

type turnContextKey struct{}
type turnContext struct {
	sessionID  string
	messageID  string
	ordinal    int64
	workingDir string
}

type fileSnapshot struct {
	FileID   string `json:"file_id,omitempty"`
	Exists   bool   `json:"exists"`
	Binary   bool   `json:"binary,omitempty"`
	Oversize bool   `json:"oversize,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Version  int64  `json:"version"`
	Content  string `json:"-"`
}

// CaptureBefore must run after permission has been granted and before writing.
// Services without turn recording retain their existing version behavior.
func CaptureBefore(ctx context.Context, files Service, path string) error {
	recorder, ok := files.(interface {
		CaptureTurnPath(context.Context, string) error
	})
	if !ok {
		return nil
	}
	return recorder.CaptureTurnPath(ctx, path)
}

func (s *service) BeginTurn(ctx context.Context, sessionID, messageID, workingDir string) (context.Context, error) {
	messages, err := s.q.ListMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	ordinal := int64(-1)
	found := false
	for _, msg := range messages {
		if msg.Role == "user" {
			ordinal++
		}
		if msg.ID == messageID {
			if msg.Role != "user" {
				return nil, fmt.Errorf("turn message is not a user message")
			}
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("turn message does not belong to session")
	}
	if err := s.q.CreateSessionTurn(ctx, db.CreateSessionTurnParams{SessionID: sessionID, TurnID: ordinal, UserMessageID: messageID}); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, turnContextKey{}, turnContext{sessionID: sessionID, messageID: messageID, ordinal: ordinal, workingDir: workingDir}), nil
}

func (s *service) CaptureTurnPath(ctx context.Context, path string) error {
	turn, ok := ctx.Value(turnContextKey{}).(turnContext)
	if !ok {
		return nil
	}
	path, err := absoluteTurnPath(turn.workingDir, path)
	if err != nil {
		return err
	}
	before, err := readTurnSnapshot(path)
	if err != nil {
		return fmt.Errorf("capture file before edit: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	_, err = qtx.GetTurnFile(ctx, db.GetTurnFileParams{SessionID: turn.sessionID, UserMessageID: turn.messageID, Path: path})
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	before.Version = turn.ordinal * 2
	if err := storeTurnSnapshot(ctx, qtx, turn.sessionID, path, &before); err != nil {
		return err
	}
	encoded, err := json.Marshal(before)
	if err != nil {
		return err
	}
	if err := qtx.InsertTurnFile(ctx, db.InsertTurnFileParams{SessionID: turn.sessionID, UserMessageID: turn.messageID, Path: path, StartState: string(encoded)}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *service) CompleteTurn(ctx context.Context) error {
	turn, ok := ctx.Value(turnContextKey{}).(turnContext)
	if !ok {
		return nil
	}
	recorded, err := s.q.GetSessionTurnByMessage(ctx, db.GetSessionTurnByMessageParams{SessionID: turn.sessionID, UserMessageID: turn.messageID})
	if err != nil {
		return err
	}
	if recorded.Completed != 0 {
		return nil
	}
	files, err := s.q.ListTurnFiles(ctx, db.ListTurnFilesParams{SessionID: turn.sessionID, UserMessageID: turn.messageID})
	if err != nil {
		return err
	}
	after := make([]fileSnapshot, len(files))
	for i, file := range files {
		after[i], err = readTurnSnapshot(file.Path)
		if err != nil {
			return fmt.Errorf("capture file after turn: %w", err)
		}
		after[i].Version = turn.ordinal*2 + 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	for i, file := range files {
		if err := storeTurnSnapshot(ctx, qtx, turn.sessionID, file.Path, &after[i]); err != nil {
			return err
		}
		encoded, err := json.Marshal(after[i])
		if err != nil {
			return err
		}
		if err := qtx.CompleteTurnFile(ctx, db.CompleteTurnFileParams{SessionID: turn.sessionID, UserMessageID: turn.messageID, Path: file.Path, EndState: sql.NullString{String: string(encoded), Valid: true}}); err != nil {
			return err
		}
	}
	if err := qtx.CompleteSessionTurn(ctx, db.CompleteSessionTurnParams{SessionID: turn.sessionID, UserMessageID: turn.messageID}); err != nil {
		return err
	}
	return tx.Commit()
}

func readTurnSnapshot(path string) (fileSnapshot, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("file history target is not a regular file: %s", path)
	}
	state := fileSnapshot{Exists: true}
	if info.Size() > maxTurnFileBytes {
		state.Oversize = true
		state.Hash = fmt.Sprintf("oversize:%d:%d", info.Size(), info.ModTime().UnixNano())
		return state, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxTurnFileBytes+1))
	if err != nil {
		return fileSnapshot{}, err
	}
	if len(content) > maxTurnFileBytes {
		return fileSnapshot{}, fmt.Errorf("file grew beyond snapshot bound: %s", path)
	}
	digest := sha256.Sum256(content)
	state.Hash = hex.EncodeToString(digest[:])
	state.Binary = !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00')
	if !state.Binary {
		state.Content = string(content)
	}
	return state, nil
}

func storeTurnSnapshot(ctx context.Context, q *db.Queries, sessionID, path string, state *fileSnapshot) error {
	if !state.Exists || state.Binary || state.Oversize {
		return nil
	}
	latest, err := q.GetFileByPathAndSession(ctx, db.GetFileByPathAndSessionParams{SessionID: sessionID, Path: path})
	next := int64(0)
	if err == nil {
		if latest.Content == state.Content {
			state.FileID, state.Version = latest.ID, latest.Version
			return nil
		}
		next = latest.Version + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	stored, err := q.CreateFile(ctx, db.CreateFileParams{ID: uuid.NewString(), SessionID: sessionID, Path: path, Content: state.Content, Version: next})
	if err != nil {
		return err
	}
	state.FileID, state.Version = stored.ID, stored.Version
	return nil
}

func (s *service) ListTurns(ctx context.Context, sessionID string) ([]Turn, error) {
	stored, err := s.q.ListSessionTurns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]Turn, 0, len(stored))
	for _, turn := range stored {
		result = append(result, Turn{TurnID: turn.TurnID, UserMessageID: turn.UserMessageID, Recorded: turn.Completed != 0})
	}
	return result, nil
}

func (s *service) TurnChanges(ctx context.Context, sessionID string, ordinal int64, workingDir string) (TurnChanges, error) {
	result := TurnChanges{Changes: []TurnChange{}}
	turn, err := s.q.GetSessionTurn(ctx, db.GetSessionTurnParams{SessionID: sessionID, TurnID: ordinal})
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if turn.Completed == 0 {
		return result, nil
	}
	files, err := s.q.ListTurnFiles(ctx, db.ListTurnFilesParams{SessionID: sessionID, UserMessageID: turn.UserMessageID})
	if err != nil {
		return result, err
	}
	for _, file := range files {
		before, err := s.decodeSnapshot(ctx, file.StartState)
		if err != nil {
			return result, err
		}
		if !file.EndState.Valid {
			return result, fmt.Errorf("completed turn is missing its ending snapshot")
		}
		after, err := s.decodeSnapshot(ctx, file.EndState.String)
		if err != nil {
			return result, err
		}
		if before.Exists == after.Exists && before.Hash == after.Hash {
			continue
		}
		change := TurnChange{Path: displayTurnPath(workingDir, file.Path), Status: "modified", Binary: before.Binary || after.Binary, Oversize: before.Oversize || after.Oversize}
		if !before.Exists {
			change.Status = "added"
		} else if !after.Exists {
			change.Status = "deleted"
		}
		if !change.Binary && !change.Oversize {
			_, change.Additions, change.Deletions = diff.GenerateDiff(before.Content, after.Content, change.Path)
		}
		result.Changes = append(result.Changes, change)
	}
	result.Recorded = true
	return result, nil
}

func (s *service) TurnContent(ctx context.Context, sessionID string, ordinal int64, path, phase, workingDir string) (*TurnContent, error) {
	if phase == "" {
		phase = "start"
	}
	if phase != "start" && phase != "end" {
		return nil, fmt.Errorf("phase must be start or end")
	}
	path, err := absoluteTurnPath(workingDir, path)
	if err != nil {
		return nil, err
	}
	turn, err := s.q.GetSessionTurn(ctx, db.GetSessionTurnParams{SessionID: sessionID, TurnID: ordinal})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file, err := s.q.GetTurnFile(ctx, db.GetTurnFileParams{SessionID: sessionID, UserMessageID: turn.UserMessageID, Path: path})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	encoded := file.StartState
	if phase == "end" {
		if !file.EndState.Valid {
			return nil, nil
		}
		encoded = file.EndState.String
	}
	state, err := s.decodeSnapshot(ctx, encoded)
	if err != nil {
		return nil, err
	}
	if !state.Exists {
		return nil, nil
	}
	content := &TurnContent{Version: state.Version, Binary: state.Binary}
	if !state.Binary && !state.Oversize {
		content.Content = &state.Content
	}
	return content, nil
}

func (s *service) decodeSnapshot(ctx context.Context, encoded string) (fileSnapshot, error) {
	var state fileSnapshot
	if err := json.Unmarshal([]byte(encoded), &state); err != nil {
		return state, err
	}
	if state.FileID != "" {
		file, err := s.q.GetFile(ctx, state.FileID)
		if err != nil {
			return state, fmt.Errorf("read retained file snapshot: %w", err)
		}
		state.Content = file.Content
	} else if state.Exists && !state.Binary && !state.Oversize {
		return state, fmt.Errorf("text snapshot has no retained file version")
	}
	return state, nil
}

func absoluteTurnPath(workingDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("file path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	return filepath.Abs(filepath.Clean(path))
}

func displayTurnPath(workingDir, path string) string {
	rel, err := filepath.Rel(workingDir, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return path
}

// CloneTurns copies retained file versions and remaps checkpoint messages into
// the same transaction used to create a forked conversation.
func CloneTurns(ctx context.Context, q *db.Queries, sourceID, newID string, messageIDs map[string]string) error {
	files, err := q.ListFilesBySession(ctx, sourceID)
	if err != nil {
		return err
	}
	fileIDs := make(map[string]string, len(files))
	for _, file := range files {
		id := uuid.NewString()
		if _, err := q.CreateFile(ctx, db.CreateFileParams{ID: id, SessionID: newID, Path: file.Path, Content: file.Content, Version: file.Version}); err != nil {
			return err
		}
		fileIDs[file.ID] = id
	}
	turns, err := q.ListSessionTurns(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		messageID, ok := messageIDs[turn.UserMessageID]
		if !ok {
			continue
		}
		if err := q.CreateSessionTurn(ctx, db.CreateSessionTurnParams{SessionID: newID, TurnID: turn.TurnID, UserMessageID: messageID, Completed: turn.Completed}); err != nil {
			return err
		}
		entries, err := q.ListTurnFiles(ctx, db.ListTurnFilesParams{SessionID: sourceID, UserMessageID: turn.UserMessageID})
		if err != nil {
			return err
		}
		remap := func(encoded string) (string, error) {
			var state fileSnapshot
			if err := json.Unmarshal([]byte(encoded), &state); err != nil {
				return "", err
			}
			if state.FileID != "" {
				id, ok := fileIDs[state.FileID]
				if !ok {
					return "", fmt.Errorf("fork snapshot file version is missing")
				}
				state.FileID = id
			}
			result, err := json.Marshal(state)
			return string(result), err
		}
		for _, entry := range entries {
			start, err := remap(entry.StartState)
			if err != nil {
				return err
			}
			end := sql.NullString{}
			if entry.EndState.Valid {
				value, err := remap(entry.EndState.String)
				if err != nil {
					return err
				}
				end = sql.NullString{String: value, Valid: true}
			}
			if err := q.InsertTurnFile(ctx, db.InsertTurnFileParams{SessionID: newID, UserMessageID: messageID, Path: entry.Path, StartState: start, EndState: end}); err != nil {
				return err
			}
		}
	}
	return nil
}

// CarryTurn retains a recorded turn when a model step supplies a fresh context.
func CarryTurn(ctx, recorded context.Context) context.Context {
	if recorded == nil {
		return ctx
	}
	if turn, ok := recorded.Value(turnContextKey{}).(turnContext); ok {
		return context.WithValue(ctx, turnContextKey{}, turn)
	}
	return ctx
}
