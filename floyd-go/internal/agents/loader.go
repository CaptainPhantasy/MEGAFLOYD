// Package agents provides loading and parsing of agent definition files.
// Agent files are markdown files with YAML frontmatter that define AI agent personas.
package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Category constants for agent classification.
const (
	CategoryCoding         = "coding"
	CategoryArchitecture   = "architecture"
	CategoryDebugging      = "debugging"
	CategoryOrchestration  = "orchestration"
	CategoryQuality        = "quality"
	CategorySecurity       = "security"
	CategoryMonitoring     = "monitoring"
	CategoryTesting        = "testing"
	CategoryDocumentation  = "documentation"
	CategoryInfrastructure = "infrastructure"
	CategoryData           = "data"
	CategoryPerformance    = "performance"
	CategoryDX             = "dx"
)

// AgentDefinition represents a parsed agent markdown file.
type AgentDefinition struct {
	// Required fields
	Name        string `yaml:"name"`        // Unique identifier for the agent
	Description string `yaml:"description"` // Human-readable description
	Category    string `yaml:"category"`    // One of 13 predefined categories

	// Optional fields
	Trigger      string   `yaml:"trigger,omitempty"` // Keyword to invoke the agent
	Version      string   `yaml:"version,omitempty"` // Agent version (semver recommended)
	Author       string   `yaml:"author,omitempty"`  // Agent author
	Tags         []string `yaml:"tags,omitempty"`    // Classification tags
	SystemPrompt string   `yaml:"-"`                 // Markdown body (system prompt content)
	FilePath     string   `yaml:"-"`                 // Absolute path to the agent file
}

// Validate checks that required fields are present.
func (a *AgentDefinition) Validate() error {
	var errs []error

	if a.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if a.Description == "" {
		errs = append(errs, errors.New("description is required"))
	}

	return errors.Join(errs...)
}

// ParseAgentFile reads and parses an agent definition from a markdown file.
// The file must contain YAML frontmatter (delimited by ---) followed by markdown content.
// Required frontmatter fields: name, description
// Optional frontmatter fields: trigger, version, author, tags, category
func ParseAgentFile(path string) (AgentDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return AgentDefinition{}, fmt.Errorf("reading agent file: %w", err)
	}

	frontmatter, body, err := splitFrontmatter(string(content))
	if err != nil {
		return AgentDefinition{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	var agent AgentDefinition
	if err := yaml.Unmarshal([]byte(frontmatter), &agent); err != nil {
		return AgentDefinition{}, fmt.Errorf("parsing frontmatter in %s: %w", path, err)
	}

	agent.SystemPrompt = strings.TrimSpace(body)
	agent.FilePath = path

	return agent, nil
}

// LoadAgents loads all .md files from the specified directory and parses them as agent definitions.
// Returns a slice of valid agent definitions. Files that fail validation are skipped.
// Returns an error only if the directory cannot be read.
func LoadAgents(dir string) ([]AgentDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return empty list if directory doesn't exist
		}
		return nil, fmt.Errorf("reading agents directory: %w", err)
	}

	var agents []AgentDefinition
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .md files
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		// Skip template files (prefixed with _)
		if strings.HasPrefix(entry.Name(), "_") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		agent, err := ParseAgentFile(path)
		if err != nil {
			// Skip files that can't be parsed (e.g., not valid agent files)
			continue
		}

		// Skip agents that fail validation
		if err := agent.Validate(); err != nil {
			continue
		}

		agents = append(agents, agent)
	}

	return agents, nil
}

// splitFrontmatter extracts YAML frontmatter and body from markdown content.
// Frontmatter must be at the start of the content, delimited by --- markers.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	// Normalize line endings to \n for consistent parsing
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Must start with ---\n
	if !strings.HasPrefix(content, "---\n") {
		return "", "", errors.New("no YAML frontmatter found (must start with ---)")
	}

	// Remove opening ---
	rest := strings.TrimPrefix(content, "---\n")

	// Find closing ---
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", "", errors.New("unclosed frontmatter (missing closing ---)")
	}

	frontmatter = rest[:idx]
	body = rest[idx+4:] // Skip past "\n---"

	return frontmatter, body, nil
}
