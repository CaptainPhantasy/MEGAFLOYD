package hooks

import "github.com/legacy-ai/floyd/internal/config"

// Semantics describes native authority; hook JSON cannot extend it.
type Semantics struct {
	CanApprove    bool `json:"can_approve"`
	CanBlock      bool `json:"can_block"`
	CanRewrite    bool `json:"can_rewrite"`
	CanRequire    bool `json:"can_require"`
	CanAsync      bool `json:"can_async"`
	Observational bool `json:"observational"`
}

func EventNames() []string {
	return []string{EventPreToolUse, EventPostToolUse, EventPostToolUseFailure, EventActionRejected, EventToolCancelled, EventPermissionRequest, EventPermissionResolved, EventSessionStart, EventStop}
}

func EventSemantics(event string) (Semantics, bool) {
	canonical, err := config.NormalizeHookEvent(event)
	if err != nil {
		return Semantics{}, false
	}
	switch canonical {
	case EventPreToolUse:
		return Semantics{CanBlock: true, CanApprove: true, CanRewrite: true, CanRequire: true}, true
	case EventPostToolUse:
		return Semantics{CanRequire: true, CanAsync: true, Observational: true}, true
	case EventSessionStart:
		return Semantics{Observational: true}, true
	default:
		return Semantics{CanAsync: true, Observational: true}, true
	}
}

func ObservationalEvent(event string) bool {
	s, ok := EventSemantics(event)
	return ok && s.Observational
}
