// Package tools provides AI agent tool implementations.
package tools

import (
	"context"
	"time"

	"github.com/legacy-ai/floyd/internal/pubsub"
)

// ToolProgressEvent represents a progress update during tool execution.
type ToolProgressEvent struct {
	ToolCallID string    // ID of the tool call this progress relates to
	Status     string    // Current status (e.g., "started", "in_progress", "completed")
	Message    string    // Human-readable progress message
	Percent    int       // Progress percentage (0-100)
	Timestamp  time.Time // When the event occurred
}

// progressBroker is the global broker for tool progress events.
// UI components subscribe to this to receive real-time progress updates;
// internal/app republishes every event onto the app event broker so
// local and client/server UIs both receive it.
var progressBroker = pubsub.NewBroker[ToolProgressEvent]()

// SubscribeProgress returns a channel that receives tool progress events.
// This is used by the UI to display real-time progress during long-running tool operations.
func SubscribeProgress(ctx context.Context) <-chan pubsub.Event[ToolProgressEvent] {
	return progressBroker.Subscribe(ctx)
}

// ProgressCallback is the signature for progress event callbacks with tool call ID.
type ProgressCallback func(toolCallID string, event ToolProgressEvent)

// progressCallback is the package-level callback for progress integration.
var progressCallback ProgressCallback

// SetProgressCallback sets the package-level progress callback for progress integration.
func SetProgressCallback(cb ProgressCallback) {
	progressCallback = cb
}

// PublishProgress publishes a tool progress event to the global broker.
// This allows UI components to receive real-time progress updates.
func PublishProgress(event ToolProgressEvent) {
	progressBroker.Publish(pubsub.UpdatedEvent, event)
}

// ShutdownProgressBroker shuts down the global progress broker.
// This should be called during application shutdown.
func ShutdownProgressBroker() {
	progressBroker.Shutdown()
}

// ProgressEmitter streams progress updates during long-running tool operations.
type ProgressEmitter struct {
	toolCallID string
	callback   func(ToolProgressEvent)
	start      time.Time
}

// NewProgressEmitter creates a new progress emitter with the given tool call ID and callback.
// If callback is nil, all Emit calls become no-ops.
func NewProgressEmitter(toolCallID string, callback func(ToolProgressEvent)) *ProgressEmitter {
	return &ProgressEmitter{
		toolCallID: toolCallID,
		callback:   callback,
		start:      time.Now(),
	}
}

// Emit sends a progress event with the given message and percentage.
// If the callback is nil, this is a no-op.
func (p *ProgressEmitter) Emit(message string, percent int) {
	if p.callback == nil {
		return
	}

	// Clamp percent to valid range
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}

	event := ToolProgressEvent{
		ToolCallID: p.toolCallID,
		Status:     "in_progress",
		Message:    message,
		Percent:    percent,
		Timestamp:  time.Now(),
	}

	p.callback(event)
}

// EmitStart emits a progress event indicating the operation has started (0%).
func (p *ProgressEmitter) EmitStart(message string) {
	if p.callback == nil {
		return
	}

	event := ToolProgressEvent{
		ToolCallID: p.toolCallID,
		Status:     "started",
		Message:    message,
		Percent:    0,
		Timestamp:  time.Now(),
	}

	p.callback(event)
}

// EmitComplete emits a progress event indicating the operation has completed (100%).
func (p *ProgressEmitter) EmitComplete(message string) {
	if p.callback == nil {
		return
	}

	event := ToolProgressEvent{
		ToolCallID: p.toolCallID,
		Status:     "completed",
		Message:    message,
		Percent:    100,
		Timestamp:  time.Now(),
	}

	p.callback(event)
}

// Elapsed returns the time since the emitter was created.
func (p *ProgressEmitter) Elapsed() time.Duration {
	return time.Since(p.start)
}
