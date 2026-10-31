package server

import (
	"net/http"

	"github.com/legacy-ai/floyd/internal/proto"
)

func (c *controllerV1) handleSessionRewind(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	request := proto.SessionUndoRequest{Count: 1}
	if !decodeSessionOperation(w, r, &request, true) {
		return
	}
	result, err := c.backend.RewindSession(r.Context(), r.PathValue("id"), r.PathValue("sid"), request.Count)
	if err != nil {
		c.sessionOperationError(w, r, err)
		return
	}
	jsonEncode(w, result)
}
