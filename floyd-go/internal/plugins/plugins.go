// Package plugins implements the Floyd Plugins system.
// Plugins bundle skills, slash commands, sub-agent definitions, and MCP connector references
// into cohesive capability packages similar to Claude's plugin architecture.
package plugins

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/charlievieth/fastwalk"
	"gopkg.in/yaml.v3"
)

const (
	PluginFileName         = "PLUGIN.md"
	MaxNameLength          = 64
	MaxDescriptionLength   = 2048
	MaxCompatibilityLength = 500
	MaxVersionLength       = 32
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)

// SlashCommand defines a custom shortcut that triggers specific automated actions.
type SlashCommand struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Trigger     string `yaml:"trigger" json:"trigger"`   // e.g., "/create-pr", "/audit"
	Template    string `yaml:"template" json:"template"` // Prompt template to inject
	AutoExecute bool   `yaml:"auto_execute" json:"auto_execute"`
}

// SubAgentDef defines a specialized mini-instance configuration.
type SubAgentDef struct {
	Name         string   `yaml:"name" json:"name"`
	Description  string   `yaml:"description" json:"description"`
	SystemPrompt string   `yaml:"system_prompt" json:"system_prompt"`
	Tools        []string `yaml:"tools" json:"tools"`           // Tool whitelist
	MCPs         []string `yaml:"mcps" json:"mcps"`             // MCP server whitelist
	ModelType    string   `yaml:"model_type" json:"model_type"` // "large" or "small"
}

// ConnectorRef references an MCP connector configuration.
type ConnectorRef struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Type        string            `yaml:"type" json:"type"` // "required" or "optional"
	Config      map[string]string `yaml:"config" json:"config"`
}

// Plugin represents a parsed PLUGIN.md file.
type Plugin struct {
	// Core metadata
	Name          string            `yaml:"name" json:"name"`
	Version       string            `yaml:"version,omitempty" json:"version,omitempty"`
	Description   string            `yaml:"description" json:"description"`
	License       string            `yaml:"license,omitempty" json:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Author        string            `yaml:"author,omitempty" json:"author,omitempty"`
	Repository    string            `yaml:"repository,omitempty" json:"repository,omitempty"`
	Category      string            `yaml:"category,omitempty" json:"category,omitempty"` // e.g., "development", "finance", "productivity"
	Tags          []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	// Plugin components
	Instructions  string         `yaml:"-" json:"instructions"` // Skill-like instructions from body
	SlashCommands []SlashCommand `yaml:"slash_commands,omitempty" json:"slash_commands,omitempty"`
	SubAgents     []SubAgentDef  `yaml:"sub_agents,omitempty" json:"sub_agents,omitempty"`
	Connectors    []ConnectorRef `yaml:"connectors,omitempty" json:"connectors,omitempty"`

	// Internal
	Path           string `yaml:"-" json:"path"`
	PluginFilePath string `yaml:"-" json:"plugin_file_path"`
	Enabled        bool   `yaml:"-" json:"enabled"`
}

// Validate checks if the plugin meets spec requirements.
func (p *Plugin) Validate() error {
	var errs []error

	if p.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else {
		if len(p.Name) > MaxNameLength {
			errs = append(errs, fmt.Errorf("name exceeds %d characters", MaxNameLength))
		}
		if !namePattern.MatchString(p.Name) {
			errs = append(errs, errors.New("name must be alphanumeric with hyphens, no leading/trailing/consecutive hyphens"))
		}
		if p.Path != "" && !strings.EqualFold(filepath.Base(p.Path), p.Name) {
			slog.Debug("Plugin directory name differs from plugin name", "name", p.Name, "directory", filepath.Base(p.Path))
		}
	}

	if p.Description == "" {
		errs = append(errs, errors.New("description is required"))
	} else if len(p.Description) > MaxDescriptionLength {
		errs = append(errs, fmt.Errorf("description exceeds %d characters", MaxDescriptionLength))
	}

	if len(p.Version) > MaxVersionLength {
		errs = append(errs, fmt.Errorf("version exceeds %d characters", MaxVersionLength))
	}

	if len(p.Compatibility) > MaxCompatibilityLength {
		errs = append(errs, fmt.Errorf("compatibility exceeds %d characters", MaxCompatibilityLength))
	}

	// Validate slash commands
	for i, cmd := range p.SlashCommands {
		if cmd.Name == "" {
			errs = append(errs, fmt.Errorf("slash_commands[%d]: name is required", i))
		}
		if cmd.Trigger == "" {
			errs = append(errs, fmt.Errorf("slash_commands[%d]: trigger is required", i))
		}
		if !strings.HasPrefix(cmd.Trigger, "/") {
			errs = append(errs, fmt.Errorf("slash_commands[%d]: trigger must start with /", i))
		}
	}

	// Validate sub-agents
	for i, agent := range p.SubAgents {
		if agent.Name == "" {
			errs = append(errs, fmt.Errorf("sub_agents[%d]: name is required", i))
		}
	}

	// Validate connectors
	for i, conn := range p.Connectors {
		if conn.Name == "" {
			errs = append(errs, fmt.Errorf("connectors[%d]: name is required", i))
		}
	}

	return errors.Join(errs...)
}

// Parse parses a PLUGIN.md file.
func Parse(path string) (*Plugin, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	frontmatter, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, err
	}

	var plugin Plugin
	if err := yaml.Unmarshal([]byte(frontmatter), &plugin); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	plugin.Instructions = strings.TrimSpace(body)
	plugin.Path = filepath.Dir(path)
	plugin.PluginFilePath = path
	plugin.Enabled = true

	return &plugin, nil
}

// splitFrontmatter extracts YAML frontmatter and body from markdown content.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	// Normalize line endings to \n for consistent parsing.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return "", "", errors.New("no YAML frontmatter found")
	}

	rest := strings.TrimPrefix(content, "---\n")
	before, after, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", "", errors.New("unclosed frontmatter")
	}

	return before, after, nil
}

// Discover finds all valid plugins in the given paths.
func Discover(paths []string) []*Plugin {
	var plugins []*Plugin
	var mu sync.Mutex
	seen := make(map[string]bool)

	for _, base := range paths {
		conf := fastwalk.Config{
			Follow:  true,
			ToSlash: fastwalk.DefaultToSlash(),
		}
		fastwalk.Walk(&conf, base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || d.Name() != PluginFileName {
				return nil
			}
			mu.Lock()
			if seen[path] {
				mu.Unlock()
				return nil
			}
			seen[path] = true
			mu.Unlock()
			plugin, err := Parse(path)
			if err != nil {
				slog.Warn("Failed to parse plugin file", "path", path, "error", err)
				return nil
			}
			if err := plugin.Validate(); err != nil {
				slog.Warn("Plugin validation failed", "path", path, "error", err)
				return nil
			}
			slog.Debug("Successfully loaded plugin", "name", plugin.Name, "path", path)
			mu.Lock()
			plugins = append(plugins, plugin)
			mu.Unlock()
			return nil
		})
	}

	return plugins
}

// ToPromptXML generates XML for injection into the system prompt.
func ToPromptXML(plugins []*Plugin) string {
	if len(plugins) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<available_plugins>\n")
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		sb.WriteString("  <plugin>\n")
		fmt.Fprintf(&sb, "    <name>%s</name>\n", escape(p.Name))
		fmt.Fprintf(&sb, "    <description>%s</description>\n", escape(p.Description))
		fmt.Fprintf(&sb, "    <location>%s</location>\n", escape(p.PluginFilePath))
		if p.Version != "" {
			fmt.Fprintf(&sb, "    <version>%s</version>\n", escape(p.Version))
		}
		if p.Category != "" {
			fmt.Fprintf(&sb, "    <category>%s</category>\n", escape(p.Category))
		}

		// Include slash commands
		if len(p.SlashCommands) > 0 {
			sb.WriteString("    <slash_commands>\n")
			for _, cmd := range p.SlashCommands {
				sb.WriteString("      <command>\n")
				fmt.Fprintf(&sb, "        <name>%s</name>\n", escape(cmd.Name))
				fmt.Fprintf(&sb, "        <trigger>%s</trigger>\n", escape(cmd.Trigger))
				fmt.Fprintf(&sb, "        <description>%s</description>\n", escape(cmd.Description))
				sb.WriteString("      </command>\n")
			}
			sb.WriteString("    </slash_commands>\n")
		}

		// Include sub-agent definitions
		if len(p.SubAgents) > 0 {
			sb.WriteString("    <sub_agents>\n")
			for _, agent := range p.SubAgents {
				sb.WriteString("      <agent>\n")
				fmt.Fprintf(&sb, "        <name>%s</name>\n", escape(agent.Name))
				fmt.Fprintf(&sb, "        <description>%s</description>\n", escape(agent.Description))
				sb.WriteString("      </agent>\n")
			}
			sb.WriteString("    </sub_agents>\n")
		}

		// Include connector references
		if len(p.Connectors) > 0 {
			sb.WriteString("    <connectors>\n")
			for _, conn := range p.Connectors {
				sb.WriteString("      <connector>\n")
				fmt.Fprintf(&sb, "        <name>%s</name>\n", escape(conn.Name))
				fmt.Fprintf(&sb, "        <type>%s</type>\n", escape(conn.Type))
				sb.WriteString("      </connector>\n")
			}
			sb.WriteString("    </connectors>\n")
		}

		sb.WriteString("  </plugin>\n")
	}
	sb.WriteString("</available_plugins>")
	return sb.String()
}

// ToInstructionsXML generates XML with just the plugin instructions for prompt injection.
// This is the core content that extends Floyd's capabilities.
func ToInstructionsXML(plugins []*Plugin) string {
	if len(plugins) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<plugin_instructions>\n")
	for _, p := range plugins {
		if !p.Enabled || p.Instructions == "" {
			continue
		}
		sb.WriteString("  <instruction plugin=\"")
		sb.WriteString(escape(p.Name))
		sb.WriteString("\">\n")
		sb.WriteString(indent(p.Instructions, "    "))
		sb.WriteString("\n  </instruction>\n")
	}
	sb.WriteString("</plugin_instructions>")
	return sb.String()
}

// GetSlashCommands returns all slash commands from all plugins.
func GetSlashCommands(plugins []*Plugin) []SlashCommand {
	var commands []SlashCommand
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		commands = append(commands, p.SlashCommands...)
	}
	return commands
}

// GetSlashCommandByTrigger finds a slash command by its trigger.
func GetSlashCommandByTrigger(plugins []*Plugin, trigger string) *SlashCommand {
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		for _, cmd := range p.SlashCommands {
			if cmd.Trigger == trigger {
				return &cmd
			}
		}
	}
	return nil
}

// GetSubAgent finds a sub-agent definition by name.
func GetSubAgent(plugins []*Plugin, name string) *SubAgentDef {
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		for _, agent := range p.SubAgents {
			if agent.Name == name {
				return &agent
			}
		}
	}
	return nil
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}
