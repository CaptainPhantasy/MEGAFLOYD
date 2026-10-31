package app

import (
	"github.com/legacy-ai/floyd/internal/agent/notify"
	"github.com/legacy-ai/floyd/internal/pubsub"
)

// The coordinator publishes completion before returning. Drain unrelated events
// already in this subscriber's queue, but never infer success from their presence.
func takeRunCompletion(events <-chan pubsub.Event[notify.RunComplete], sessionID, runID string) (notify.RunComplete, bool) {
	for {
		select {
		case event, open := <-events:
			if !open {
				return notify.RunComplete{}, false
			}
			if event.Payload.SessionID == sessionID && event.Payload.RunID == runID {
				return event.Payload, true
			}
		default:
			return notify.RunComplete{}, false
		}
	}
}
