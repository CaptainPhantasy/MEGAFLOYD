package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/history"
	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/session"
)

func (c *controllerV1) registerSessionOperationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/sessions/{sid}/settings", c.handleSessionSettings)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/settings", c.handleUpdateSessionSettings)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/archive", c.handleSessionArchive)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/metadata", c.handleSessionMetadata)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/fork", c.handleSessionFork)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/branch", c.handleSessionBranch)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/undo", c.handleSessionUndo)
	mux.HandleFunc("GET /v1/workspaces/{id}/sessions/{sid}/file-history/turns", c.handleSessionHistoryTurns)
	mux.HandleFunc("GET /v1/workspaces/{id}/sessions/{sid}/file-history/changes", c.handleSessionHistoryChanges)
	mux.HandleFunc("GET /v1/workspaces/{id}/sessions/{sid}/file-history/content", c.handleSessionHistoryContent)
}

func (c *controllerV1) sessionOperationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		jsonError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, agent.ErrSessionBusy):
		jsonError(w, http.StatusConflict, err.Error())
	case errors.Is(err, agent.ErrInvalidSessionSettings):
		jsonError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrInvalidUndo):
		jsonError(w, http.StatusBadRequest, err.Error())
	default:
		c.handleError(w, r, err)
	}
}

func decodeSessionOperation(w http.ResponseWriter, r *http.Request, target any, optional bool) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return true
		}
		jsonError(w, http.StatusBadRequest, "invalid session operation request")
		return false
	}
	return true
}

func (c *controllerV1) encodeSessionOperation(w http.ResponseWriter, r *http.Request, sess session.Session) {
	result := sessionToProto(sess)
	ws, _ := c.backend.GetWorkspace(r.PathValue("id"))
	result.IsBusy = isSessionBusy(ws, sess.ID)
	result.AttachedClients = attachedClients(ws, sess.ID)
	jsonEncode(w, result)
}

func (c *controllerV1) handleSessionArchive(w http.ResponseWriter, r *http.Request) {
	var request proto.SessionArchiveRequest
	if !decodeSessionOperation(w, r, &request, false) {
		return
	}
	if request.Archived == nil {
		jsonError(w, http.StatusBadRequest, "archived is required")
		return
	}
	sess, err := c.backend.SetSessionArchived(r.Context(), r.PathValue("id"), r.PathValue("sid"), *request.Archived)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	c.encodeSessionOperation(w, r, sess)
}

func (c *controllerV1) handleSessionMetadata(w http.ResponseWriter, r *http.Request) {
	var request proto.SessionMetadataRequest
	if !decodeSessionOperation(w, r, &request, false) {
		return
	}
	sess, err := c.backend.UpdateSessionMetadata(r.Context(), r.PathValue("id"), r.PathValue("sid"), request)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	c.encodeSessionOperation(w, r, sess)
}

func (c *controllerV1) handleSessionFork(w http.ResponseWriter, r *http.Request) {
	c.handleSessionClone(w, r, false)
}
func (c *controllerV1) handleSessionBranch(w http.ResponseWriter, r *http.Request) {
	c.handleSessionClone(w, r, true)
}
func (c *controllerV1) handleSessionClone(w http.ResponseWriter, r *http.Request, branch bool) {
	var request proto.SessionCloneRequest
	if !decodeSessionOperation(w, r, &request, true) {
		return
	}
	sess, err := c.backend.CloneSession(r.Context(), r.PathValue("id"), r.PathValue("sid"), request, branch)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	c.encodeSessionOperation(w, r, sess)
}

func (c *controllerV1) handleSessionUndo(w http.ResponseWriter, r *http.Request) {
	request := proto.SessionUndoRequest{Count: 1}
	if !decodeSessionOperation(w, r, &request, true) {
		return
	}
	if request.Count < 1 {
		jsonError(w, http.StatusBadRequest, "count must be positive")
		return
	}
	result, err := c.backend.UndoSession(r.Context(), r.PathValue("id"), r.PathValue("sid"), request.Count)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, proto.SessionUndoResponse{Session: sessionToProto(result.Session), RemovedTurns: result.RemovedTurns, UndoRecordID: result.RecordID})
}

func historyTurnOrdinal(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value, err := strconv.ParseInt(r.URL.Query().Get("turn_id"), 10, 64)
	if err != nil || value < 0 {
		jsonError(w, http.StatusBadRequest, "turn_id must be a non-negative integer")
		return 0, false
	}
	return value, true
}

func (c *controllerV1) handleSessionHistoryTurns(w http.ResponseWriter, r *http.Request) {
	turns, err := c.backend.ListSessionTurns(r.Context(), r.PathValue("id"), r.PathValue("sid"))
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, struct {
		Turns []history.Turn `json:"turns"`
	}{Turns: turns})
}

func (c *controllerV1) handleSessionHistoryChanges(w http.ResponseWriter, r *http.Request) {
	turn, ok := historyTurnOrdinal(w, r)
	if !ok {
		return
	}
	result, err := c.backend.SessionTurnChanges(r.Context(), r.PathValue("id"), r.PathValue("sid"), turn)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, result)
}

func (c *controllerV1) handleSessionHistoryContent(w http.ResponseWriter, r *http.Request) {
	turn, ok := historyTurnOrdinal(w, r)
	if !ok {
		return
	}
	path, phase := r.URL.Query().Get("path"), r.URL.Query().Get("phase")
	if path == "" || (phase != "" && phase != "start" && phase != "end") {
		jsonError(w, http.StatusBadRequest, "path and a valid phase are required")
		return
	}
	content, err := c.backend.SessionTurnContent(r.Context(), r.PathValue("id"), r.PathValue("sid"), turn, path, phase)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, struct {
		Content *history.TurnContent `json:"content"`
	}{Content: content})
}
