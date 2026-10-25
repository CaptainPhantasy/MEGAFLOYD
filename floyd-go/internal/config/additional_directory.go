package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type AdditionalDirectoryResult struct {
	ProjectRoot    string   `json:"project_root"`
	ConfigPath     string   `json:"config_path"`
	AdditionalDirs []string `json:"additional_dirs"`
	Persisted      bool     `json:"persisted"`
}

func (s *ConfigStore) AddWorkspaceDirectory(path string, persist bool) (AdditionalDirectoryResult, error) {
	if !filepath.IsAbs(path) {
		return AdditionalDirectoryResult{}, fmt.Errorf("additional directory must be an absolute path")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return AdditionalDirectoryResult{}, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return AdditionalDirectoryResult{}, err
	}
	if !info.IsDir() {
		return AdditionalDirectoryResult{}, fmt.Errorf("additional path must be a directory")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cfg := s.Config().cloneForWrite()
	if cfg.Options == nil {
		cfg.Options = &Options{}
	}
	dirs := slices.Clone(cfg.Options.AdditionalDirs)
	if !slices.Contains(dirs, canonical) {
		dirs = append(dirs, canonical)
	}
	configPath, err := s.configPath(ScopeWorkspace)
	if err != nil {
		return AdditionalDirectoryResult{}, err
	}
	if persist {
		stored := make([]string, 0, len(dirs))
		for _, dir := range dirs {
			if dir == canonical || !slices.Contains(s.overrides.AdditionalDirs, dir) {
				stored = append(stored, dir)
			}
		}
		if err := s.writeConfigFields(ScopeWorkspace, map[string]any{"options.additional_dirs": stored}); err != nil {
			return AdditionalDirectoryResult{}, err
		}
		s.overrides.AdditionalDirs = slices.DeleteFunc(slices.Clone(s.overrides.AdditionalDirs), func(dir string) bool { return dir == canonical })
		s.captureStalenessSnapshot(append(slices.Clone(s.loadedPaths), configPath))
	} else if !slices.Contains(cfg.Options.AdditionalDirs, canonical) && !slices.Contains(s.overrides.AdditionalDirs, canonical) {
		s.overrides.AdditionalDirs = append(slices.Clone(s.overrides.AdditionalDirs), canonical)
	}
	cfg.Options.AdditionalDirs = dirs
	s.setConfig(cfg)
	return AdditionalDirectoryResult{s.workingDir, configPath, slices.Clone(dirs), persist}, nil
}
