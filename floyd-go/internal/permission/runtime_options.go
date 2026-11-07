package permission

import (
	"context"
	"errors"
	"slices"
)

// RuntimeAllowlist changes ordinary permission policy only. The agent's
// protected-operation and plan checks still run before tool execution.
type RuntimeAllowlist interface{ SetAllowedTools([]string) }

func (s *permissionService) SetAllowedTools(tools []string) {
	s.allowedToolsMu.Lock()
	defer s.allowedToolsMu.Unlock()
	s.allowedTools = slices.Clone(tools)
}

// Noninteractive calls cannot wait for a human who is not attached. Explicit
// policy grants are still evaluated before this marker denies a new prompt.
type nonInteractiveKey struct{}

var ErrInteractionRequired = errors.New("permission requires an interactive confirmation; rerun in a terminal or explicitly allow this ordinary tool")

func WithNonInteractive(ctx context.Context) context.Context {
	return WithNonInteractiveMode(ctx, true)
}

func IsNonInteractive(ctx context.Context) bool {
	value, _ := ctx.Value(nonInteractiveKey{}).(bool)
	return value
}

// WithNonInteractiveMode restores an accepted call mode when a queued call inherits another run context.
func WithNonInteractiveMode(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, nonInteractiveKey{}, enabled)
}
