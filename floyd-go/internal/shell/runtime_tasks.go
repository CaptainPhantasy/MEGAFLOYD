package shell

import (
	"context"
	"fmt"
	"time"
)

type runtimeTaskOwnerKey struct{}
type runtimeTaskOwner struct{ sessionID, workspace string }

// StartForSession preserves the workspace that owns the command, even when the
// tool explicitly executes in a different working directory.
func (m *BackgroundShellManager) StartForSession(ctx context.Context, workingDir string, blocks []BlockFunc, command, description, sessionID, workspace string) (*BackgroundShell, error) {
	return m.Start(context.WithValue(ctx, runtimeTaskOwnerKey{}, runtimeTaskOwner{sessionID, workspace}), workingDir, blocks, command, description)
}
func (b *BackgroundShell) CompletedAt() time.Time {
	n := b.completedAt.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}
func (b *BackgroundShell) Cancelled() bool { return b.cancelled.Load() }
func (m *BackgroundShellManager) Cancel(id string) error {
	b, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("background shell not found: %s", id)
	}
	if !b.IsDone() {
		b.cancelled.Store(true)
		b.cancel()
	}
	return nil
}
