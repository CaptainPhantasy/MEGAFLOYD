package config

import "slices"

// UpdatePluginRuntime serializes a plugin-only edit with other configuration
// writers. Failed persistence leaves the running configuration unchanged.
func (s *ConfigStore) UpdatePluginRuntime(mutate func(*Config) (map[string]any, error)) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	next := s.Config().cloneForWrite()
	fields, err := mutate(next)
	if err != nil {
		return err
	}
	if err = s.writeConfigFields(ScopeWorkspace, fields); err != nil {
		return err
	}
	s.setConfig(next)
	if path, err := s.configPath(ScopeWorkspace); err == nil {
		s.captureStalenessSnapshot(append(slices.Clone(s.loadedPaths), path))
	}
	return nil
}
