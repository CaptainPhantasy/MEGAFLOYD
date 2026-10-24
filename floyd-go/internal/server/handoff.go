package server

import (
	"errors"
	"net/http"

	"github.com/legacy-ai/floyd/internal/megafloydhandoff"
)

func (c *controllerV1) registerHandoff(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workspaces/{id}/sessions/{sid}/handoff", c.handleHandoff)
	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/handoff", c.handleHandoff)
	mux.HandleFunc("GET /v1/workspaces/{id}/handoff", c.handleHandoff)
	mux.HandleFunc("POST /v1/workspaces/{id}/handoff", c.handleHandoff)
	mux.HandleFunc("POST /v1/handoff/web", c.handleRegisterHandoffWeb)
}
func (c *controllerV1) handoffHost() string { return c.server.network + "://" + c.server.Addr }
func (c *controllerV1) handleRegisterHandoffWeb(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL string `json:"url"`
	}
	if !decodeSessionOperation(w, r, &request, false) {
		return
	}
	if err := megafloydhandoff.RegisterWeb(r.Context(), c.handoffHost(), request.URL); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonEncode(w, map[string]bool{"registered": true})
}
func (c *controllerV1) handleHandoff(w http.ResponseWriter, r *http.Request) {
	ws, err := c.backend.GetWorkspace(r.PathValue("id"))
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	sid := r.PathValue("sid")
	if sid != "" {
		if _, err := c.backend.GetSession(r.Context(), ws.ID, sid); err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
	}
	cfg := ws.Cfg.Config()
	opts := megafloydhandoff.Options{CWD: ws.Path, DataDir: cfg.Options.DataDirectory, SessionID: sid, Host: c.handoffHost()}
	var result megafloydhandoff.Descriptor
	if r.Method == http.MethodGet {
		result, err = megafloydhandoff.Describe(r.Context(), opts)
	} else {
		var request struct {
			Target        string `json:"target"`
			Open          *bool  `json:"open,omitempty"`
			PreferredPort int    `json:"preferred_port,omitempty"`
		}
		if !decodeSessionOperation(w, r, &request, false) {
			return
		}
		if request.Target != "web" && request.Target != "tui" && request.Target != "desktop" {
			jsonError(w, http.StatusBadRequest, megafloydhandoff.ErrInvalidTarget.Error())
			return
		}
		if request.PreferredPort < 0 || request.PreferredPort > 65535 {
			jsonError(w, http.StatusBadRequest, "invalid preferred web port")
			return
		}
		if request.Target == "web" && opts.SessionID == "" {
			sessions, listErr := c.backend.ListSessions(r.Context(), ws.ID)
			if listErr != nil {
				c.handleError(w, r, listErr)
				return
			}
			var latest int64
			for _, sess := range sessions {
				if !sess.Archived && sess.ParentSessionID == "" && (opts.SessionID == "" || sess.UpdatedAt > latest) {
					opts.SessionID = sess.ID
					latest = sess.UpdatedAt
				}
			}
			if opts.SessionID == "" {
				sess, createErr := c.backend.CreateSession(r.Context(), ws.ID, "New session")
				if createErr != nil {
					c.handleError(w, r, createErr)
					return
				}
				opts.SessionID = sess.ID
			}
		}
		opts.WebPort = request.PreferredPort
		if request.Target == "web" && request.Open != nil && !*request.Open {
			result, err = megafloydhandoff.EnsureWeb(r.Context(), opts)
		} else {
			result, err = megafloydhandoff.Open(r.Context(), opts, request.Target)
		}
	}
	if err != nil {
		if errors.Is(err, megafloydhandoff.ErrInvalidTarget) {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, result)
}
