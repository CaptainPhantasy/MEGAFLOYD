package config

import "slices"

// UpdatePluginRuntime serializes a plugin-only edit with other configuration
// writers. Failed persistence leaves the running configuration unchanged.
//
// While mutate runs, this goroutine holds writeMu exclusively. The mutate
// callback therefore must NOT call any store accessor that takes writeMu's
// read lock (Resolver, Resolve, KnownProviders, Overrides, LoadedPaths):
// sync.RWMutex is not reentrant, and a read request from the writing
// goroutine blocks forever. Read what the callback needs before calling
// this method and pass it in through the closure.
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
