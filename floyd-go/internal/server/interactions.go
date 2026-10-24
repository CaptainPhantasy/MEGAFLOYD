package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/legacy-ai/floyd/internal/proto"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

func (c *controllerV1) handleGetWorkspaceInteractions(w http.ResponseWriter, r *http.Request) {
	pending, err := c.backend.PendingInteractions(r.PathValue("id"))
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, pending)
}

// Replay pending interactions after subscription so a joining client cannot
// miss a request that was published before it attached.
func (c *controllerV1) writePendingInteractions(w http.ResponseWriter, r *http.Request) {
	pending, err := c.backend.PendingInteractions(r.PathValue("id"))
	if err != nil {
		return
	}
	emit := func(payload *pubsub.Payload) {
		data, err := json.Marshal(payload)
		if err == nil {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}
	for _, req := range pending.Permissions {
		emit(envelope(pubsub.PayloadTypePermissionRequest, pubsub.Event[proto.PermissionRequest]{Type: pubsub.CreatedEvent, Payload: req}))
	}
	for _, req := range pending.Questions {
		emit(envelope(pubsub.PayloadTypeQuestionRequest, pubsub.Event[proto.QuestionRequest]{Type: pubsub.CreatedEvent, Payload: req}))
	}
	_ = http.NewResponseController(w).Flush()
}
