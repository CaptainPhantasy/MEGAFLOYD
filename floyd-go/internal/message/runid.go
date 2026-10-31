package message

import "context"

type runIDKey struct{}

// WithRunID attaches transient event provenance; persisted history is unchanged.
func WithRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, runIDKey{}, id)
}

func RunIDFromContext(ctx context.Context) string { id, _ := ctx.Value(runIDKey{}).(string); return id }
