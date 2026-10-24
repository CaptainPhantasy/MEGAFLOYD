package server

import (
	"encoding/json"
	"net/http"
)

func (c *controllerV1) handleSessionSettings(w http.ResponseWriter, r *http.Request) {
	result, err := c.backend.GetSessionRunSettings(r.Context(), r.PathValue("id"), r.PathValue("sid"))
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, result)
}

func (c *controllerV1) handleUpdateSessionSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if !decodeSessionOperation(w, r, &patch, false) {
		return
	}
	result, err := c.backend.UpdateSessionRunSettings(r.Context(), r.PathValue("id"), r.PathValue("sid"), patch)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, result)
}
