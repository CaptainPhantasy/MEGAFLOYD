package server

import (
	"encoding/json"
	"net/http"

	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *controllerV1) handlePostWorkspaceSuggestion(w http.ResponseWriter, r *http.Request) {
	var req proto.SuggestPromptRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid suggestion request")
		return
	}
	if len(req.Prefix) > 4096 {
		jsonError(w, http.StatusBadRequest, "suggestion prefix exceeds 4096 bytes")
		return
	}
	suggestion, err := c.backend.SuggestPrompt(r.Context(), r.PathValue("id"), r.PathValue("sid"), req.Prefix)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proto.SuggestPromptResponse{Suggestion: suggestion})
}
