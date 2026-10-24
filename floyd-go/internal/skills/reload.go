package skills

import (
	"github.com/legacy-ai/floyd/internal/pubsub"
	"slices"
)

// Reload retains the workspace broker so existing TUI/browser subscribers see
// the same discovery update as the engine.
func (m *Manager) Reload(cfg DiscoveryConfig) {
	all, active, states := DiscoverFromConfig(cfg)
	m.mu.Lock()
	m.allSkills = all
	m.activeSkills = active
	m.states = cloneStates(states)
	m.resolvedPaths = cfg.ResolvePaths()
	m.mu.Unlock()
	if m.globalMirror {
		SetLatestStates(states)
	}
	m.broker.Publish(pubsub.UpdatedEvent, Event{States: cloneStates(states)})
	if m.globalMirror {
		PublishStates(states)
	}
}
func (t *Tracker) Reload(active []*Skill) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeNames = make(map[string]bool, len(active))
	for _, skill := range active {
		t.activeNames[skill.Name] = true
	}
	for name := range t.loaded {
		if !t.activeNames[name] {
			delete(t.loaded, name)
		}
	}
}

func (m *Manager) pathsSnapshot() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.resolvedPaths)
}
