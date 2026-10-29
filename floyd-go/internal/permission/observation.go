package permission

import (
	"context"
	"log/slog"
)

// Observation describes the actual native permission request and its outcome.
// The callback has no return value and cannot grant or reject a request.
type Observation struct {
	Requested bool
	Request   PermissionRequest
	Granted   bool
	Cancelled bool
}

func (s *permissionService) SetLifecycleObserver(observer func(context.Context, Observation)) {
	s.observerMu.Lock()
	s.observer = observer
	s.observerMu.Unlock()
}

func (s *permissionService) observePermission(ctx context.Context, observation Observation) {
	s.observerMu.RLock()
	observer := s.observer
	s.observerMu.RUnlock()
	if observer == nil {
		return
	}
	defer func() {
		if recover() != nil {
			slog.Error("Permission observer failed")
		}
	}()
	observer(ctx, observation)
}
