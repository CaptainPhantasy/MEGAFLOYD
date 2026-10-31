package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// RuntimeOptions are explicit launch choices. They never rewrite a user's
// configuration and remain in force when that configuration is reloaded.
type RuntimeOptions struct {
	PermissionMode *string                             `json:"permission_mode,omitempty"`
	AllowedTools   []string                            `json:"allowed_tools"`
	AdditionalDirs []string                            `json:"additional_dirs,omitempty"`
	Models         map[SelectedModelType]SelectedModel `json:"models,omitempty"`
}

func (s *ConfigStore) ApplyRuntimeOptions(options RuntimeOptions) error {
	for kind, selected := range options.Models {
		if kind != SelectedModelTypeLarge && kind != SelectedModelTypeSmall {
			return fmt.Errorf("unknown model slot %q", kind)
		}
		if !s.Config().IsModelAvailable(selected.Provider, selected.Model) {
			return fmt.Errorf("model %s/%s is not configured", selected.Provider, selected.Model)
		}
	}
	if options.PermissionMode != nil {
		switch *options.PermissionMode {
		case "manual", "yolo", "auto":
		default:
			return fmt.Errorf("unknown permission mode %q", *options.PermissionMode)
		}
	}
	for _, tool := range options.AllowedTools {
		if strings.TrimSpace(tool) == "" || strings.ContainsAny(tool, "() \t\n") {
			return fmt.Errorf("invalid allowed tool %q; use an exact tool name", tool)
		}
	}
	dirs := make([]string, 0, len(options.AdditionalDirs))
	for _, path := range options.AdditionalDirs {
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.workingDir, path)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("additional directory: %w", err)
		}
		info, err := os.Stat(canonical)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("additional path %q is not a directory", path)
		}
		dirs = append(dirs, canonical)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cfg := s.Config().cloneForWrite()
	if cfg.Permissions == nil {
		cfg.Permissions = &Permissions{}
	}
	if s.overrides.Models == nil {
		s.overrides.Models = make(map[SelectedModelType]SelectedModel)
	}
	for kind, selected := range options.Models {
		cfg.Models[kind] = selected
		s.overrides.Models[kind] = selected
	}
	if options.PermissionMode != nil {
		mode := *options.PermissionMode
		s.overrides.PermissionMode = &mode
		cfg.Permissions.Mode = mode
	}
	if options.AllowedTools != nil {
		s.overrides.AllowedTools = slices.Clone(options.AllowedTools)
		cfg.Permissions.AllowedTools = slices.Clone(options.AllowedTools)
	}
	for _, dir := range dirs {
		if !slices.Contains(s.overrides.AdditionalDirs, dir) {
			s.overrides.AdditionalDirs = append(s.overrides.AdditionalDirs, dir)
		}
		if !slices.Contains(cfg.Options.AdditionalDirs, dir) {
			cfg.Options.AdditionalDirs = append(cfg.Options.AdditionalDirs, dir)
		}
	}
	s.setConfig(cfg)
	return nil
}
