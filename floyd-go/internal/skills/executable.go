package skills

import (
	"fmt"
	"path/filepath"

	"github.com/legacy-ai/floyd/internal/extensibility"
)

func parseExecutableIndex(path string) ([]*Skill, error) {
	index, err := extensibility.LoadYAMLIndex(path)
	if err != nil || index == nil {
		return nil, err
	}
	var out []*Skill
	for _, entry := range index.Skills {
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}
		if entry.Runtime == nil {
			continue
		}
		skill := &Skill{Name: entry.Name, Description: entry.Description, Runtime: entry.Runtime, UserInvocable: true, SkillFilePath: path}
		if err := skill.Validate(); err != nil {
			return nil, fmt.Errorf("skill %q: %w", entry.Name, err)
		}
		skill.Path = filepath.Dir(path)
		skill.Instructions = fmt.Sprintf("Execute this registered skill using run_skill with name %q.", entry.Name)
		out = append(out, skill)
	}
	return out, nil
}
