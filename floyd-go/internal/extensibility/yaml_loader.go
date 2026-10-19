package extensibility

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLIndex is the top-level structure for a skills.yaml configuration file.
type YAMLIndex struct {
	Version int            `yaml:"version"`
	Skills  []YAMLSkill    `yaml:"skills"`
	Agents  []YAMLSkill    `yaml:"agents,omitempty"`
	Plugins []YAMLSkill    `yaml:"plugins,omitempty"`
}

// YAMLSkill represents a single skill/agent/plugin definition in YAML.
type YAMLSkill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Path        string   `yaml:"path"`
	Category    string   `yaml:"category"` // skill, agent, plugin, command
	Tags        []string `yaml:"tags,omitempty"`
	Enabled     *bool    `yaml:"enabled,omitempty"` // nil = enabled (default)
	Runtime     *Runtime `yaml:"runtime,omitempty"`
}

// Runtime specifies how a skill is executed at runtime.
type Runtime struct {
	Type    string            `yaml:"type"` // bash, node, python, builtin
	Command string            `yaml:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Timeout int               `yaml:"timeout,omitempty"` // seconds
}

const (
	// YAMLSkillsFilename is the default filename for YAML skill configs.
	YAMLSkillsFilename = "skills.yaml"
	// YAMLVersion is the supported schema version.
	YAMLVersion = 1
)

// LoadYAMLIndex reads a skills.yaml file and returns its parsed entries.
// Returns nil if the file does not exist (not an error).
func LoadYAMLIndex(yamlPath string) (*YAMLIndex, error) {
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("read yaml index %s: %w", yamlPath, err)
	}

	var idx YAMLIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse yaml index %s: %w", yamlPath, err)
	}

	if idx.Version > YAMLVersion {
		return nil, fmt.Errorf("yaml index version %d exceeds supported version %d", idx.Version, YAMLVersion)
	}

	return &idx, nil
}

// ToEntries converts a YAMLIndex into the universal IndexEntry slice.
func (idx *YAMLIndex) ToEntries() []IndexEntry {
	var entries []IndexEntry
	for _, s := range idx.Skills {
		if s.Enabled != nil && !*s.Enabled {
			continue
		}
		entries = append(entries, IndexEntry{
			Name:        s.Name,
			Description: s.Description,
			Path:        cleanPath(s.Path),
			Category:    normalizeCategory(s.Category, "skill"),
			Tags:        s.Tags,
		})
	}
	for _, a := range idx.Agents {
		if a.Enabled != nil && !*a.Enabled {
			continue
		}
		entries = append(entries, IndexEntry{
			Name:        a.Name,
			Description: a.Description,
			Path:        cleanPath(a.Path),
			Category:    normalizeCategory(a.Category, "agent"),
			Tags:        a.Tags,
		})
	}
	for _, p := range idx.Plugins {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		entries = append(entries, IndexEntry{
			Name:        p.Name,
			Description: p.Description,
			Path:        cleanPath(p.Path),
			Category:    normalizeCategory(p.Category, "plugin"),
			Tags:        p.Tags,
		})
	}
	return entries
}

// normalizeCategory defaults the category if empty.
func normalizeCategory(cat, fallback string) string {
	cat = strings.TrimSpace(strings.ToLower(cat))
	if cat == "" {
		return fallback
	}
	return cat
}

// LoadIndexSmart loads entries preferring YAML format. It first checks
// for a skills.yaml file in the same directory as indexPath; if found,
// it uses that. Otherwise, it falls back to the legacy markdown parser.
func LoadIndexSmart(indexPath string) ([]IndexEntry, error) {
	// Try YAML first: look for skills.yaml in the same directory
	dir := filepath.Dir(indexPath)
	yamlPath := filepath.Join(dir, YAMLSkillsFilename)

	yamlIdx, err := LoadYAMLIndex(yamlPath)
	if err != nil {
		// YAML exists but failed to parse — this is an error worth surfacing
		return nil, fmt.Errorf("yaml index error (falling back to markdown): %w", err)
	}

	if yamlIdx != nil && len(yamlIdx.Skills)+len(yamlIdx.Agents)+len(yamlIdx.Plugins) > 0 {
		return yamlIdx.ToEntries(), nil
	}

	// Fallback: use the legacy markdown parser
	return LoadIndex(indexPath)
}
