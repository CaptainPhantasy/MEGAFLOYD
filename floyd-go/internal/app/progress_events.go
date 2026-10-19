package app

import (
	"context"

	"github.com/legacy-ai/floyd/internal/agent/tools"
)

// ToolProgressMsg carries a tool progress event onto the app event broker
// so UI subscribers (local TUI or client/server clients) receive live
// progress during long-running tool operations.
type ToolProgressMsg tools.ToolProgressEvent

// forwardToolProgress subscribes to the tools progress broker and
// republishes every event on the app event broker. It runs until ctx is
// canceled.
func (app *App) forwardToolProgress(ctx context.Context) {
	for event := range tools.SubscribeProgress(ctx) {
		app.SendEvent(ToolProgressMsg(event.Payload))
	}
}
