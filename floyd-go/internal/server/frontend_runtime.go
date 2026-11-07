package server

import (
	"net/http"

	"github.com/legacy-ai/floyd/internal/agent"
	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *controllerV1) registerFrontendRuntime(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workspaces/{id}/frontend-operation", func(w http.ResponseWriter, r *http.Request) {
		var in proto.FrontendRequest
		if !runtimeBody(w, r, &in) {
			return
		}
		out, err := c.backend.FrontendOperation(r.Context(), r.PathValue("id"), in)
		if err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
		jsonEncode(w, out)
	})

	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/compact", func(w http.ResponseWriter, r *http.Request) {
		var in proto.CompactRequest
		if !runtimeBody(w, r, &in) {
			return
		}
		if err := c.backend.SummarizeSession(agent.WithSummaryInstruction(r.Context(), in.Instruction), r.PathValue("id"), r.PathValue("sid")); err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
		jsonEncode(w, map[string]bool{"compacted": true})
	})

	mux.HandleFunc("POST /v1/workspaces/{id}/sessions/{sid}/ralph", func(w http.ResponseWriter, r *http.Request) {
		var in proto.RalphGoalRequest
		if !runtimeBody(w, r, &in) {
			return
		}
		state, err := c.backend.RalphGoal(r.Context(), r.PathValue("id"), r.PathValue("sid"), in)
		if err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
		jsonEncode(w, state)
	})
	mux.HandleFunc("POST /v1/workspaces/{id}/config/reload", func(w http.ResponseWriter, r *http.Request) {
		if err := c.backend.ReloadConfig(r.Context(), r.PathValue("id")); err != nil {
			c.sessionOperationError(w, r, err)
			return
		}
		jsonEncode(w, map[string]bool{"reloaded": true})
	})
}
