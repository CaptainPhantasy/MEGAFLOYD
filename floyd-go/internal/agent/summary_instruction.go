package agent

import "context"

type summaryInstructionKey struct{}

// WithSummaryInstruction carries an explicit user request through the existing
// summary operation without modifying the session's lasting system policy.
func WithSummaryInstruction(ctx context.Context, instruction string) context.Context {
	return context.WithValue(ctx, summaryInstructionKey{}, instruction)
}

func summaryInstruction(ctx context.Context) string {
	value, _ := ctx.Value(summaryInstructionKey{}).(string)
	return value
}
