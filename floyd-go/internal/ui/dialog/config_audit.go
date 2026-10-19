package dialog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/legacy-ai/floyd/internal/ui/common"
)

const (
	ConfigAuditID              = "config_audit"
	configAuditDialogMaxWidth  = 100
	configAuditDialogMaxHeight = 30
)

// ConfigAudit represents a dialog showing config audit results.
type ConfigAudit struct {
	com    *common.Common
	help   help.Model
	audit  ConfigAuditResult
	width  int
	height int

	keyMap struct {
		Close key.Binding
	}
}

// ConfigAuditResult contains the results of a config audit.
type ConfigAuditResult struct {
	GlobalConfigPath  string
	ProjectConfigPath string
	EffectiveConfig   string
	Warnings          []ConfigWarning
	DuplicateServers  []DuplicateServer
	OrphanedFiles     []string
}

// ConfigWarning represents a configuration issue.
type ConfigWarning struct {
	Level   string // "error", "warn", "info"
	Message string
	Source  string
}

// DuplicateServer represents a duplicate MCP server definition.
type DuplicateServer struct {
	Name      string
	Locations []string
}

var _ Dialog = (*ConfigAudit)(nil)

// NewConfigAudit creates a new config audit dialog.
func NewConfigAudit(com *common.Common, audit ConfigAuditResult) (*ConfigAudit, error) {
	c := &ConfigAudit{
		com:   com,
		audit: audit,
	}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	c.help = help

	c.keyMap.Close = CloseKey

	return c, nil
}

// ID implements Dialog.
func (c *ConfigAudit) ID() string {
	return ConfigAuditID
}

// HandleMsg implements [Dialog].
func (c *ConfigAudit) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, c.keyMap.Close) {
			return ActionClose{}
		}
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
	}
	return nil
}

// Cursor implements [Dialog].
func (c *ConfigAudit) Cursor() *tea.Cursor {
	return nil
}

// Draw implements [Dialog].
func (c *ConfigAudit) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := c.com.Styles
	width := max(0, min(configAuditDialogMaxWidth, area.Dx()))
	height := max(0, min(configAuditDialogMaxHeight, area.Dy()))

	var content strings.Builder

	// Title
	content.WriteString(t.Dialog.Title.Render("Config Audit Report"))
	content.WriteString("\n\n")

	// Config sources
	sectionStyle := t.Dialog.PrimaryText.Bold(true)
	content.WriteString(sectionStyle.Render("Config Sources:"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  Global:  %s\n", c.pathStatus(c.audit.GlobalConfigPath)))
	if c.audit.ProjectConfigPath != "" {
		content.WriteString(fmt.Sprintf("  Project: %s\n", c.pathStatus(c.audit.ProjectConfigPath)))
	}
	content.WriteString("\n")

	// Duplicate servers
	if len(c.audit.DuplicateServers) > 0 {
		content.WriteString(sectionStyle.Render("Duplicate MCP Servers:"))
		content.WriteString("\n")
		for _, dup := range c.audit.DuplicateServers {
			content.WriteString(fmt.Sprintf("  ⚠️  %s defined in %d locations:\n", dup.Name, len(dup.Locations)))
			for _, loc := range dup.Locations {
				content.WriteString(fmt.Sprintf("      - %s\n", loc))
			}
		}
		content.WriteString("\n")
	}

	// Warnings
	if len(c.audit.Warnings) > 0 {
		content.WriteString(sectionStyle.Render("Warnings:"))
		content.WriteString("\n")
		for _, w := range c.audit.Warnings {
			icon := "ℹ️"
			if w.Level == "error" {
				icon = "❌"
			} else if w.Level == "warn" {
				icon = "⚠️"
			}
			content.WriteString(fmt.Sprintf("  %s [%s] %s\n", icon, w.Source, w.Message))
		}
		content.WriteString("\n")
	}

	// Orphaned files
	if len(c.audit.OrphanedFiles) > 0 {
		content.WriteString(sectionStyle.Render("Orphaned Config Files:"))
		content.WriteString("\n")
		for _, f := range c.audit.OrphanedFiles {
			content.WriteString(fmt.Sprintf("  📄 %s (not loaded)\n", f))
		}
		content.WriteString("\n")
	}

	// Summary
	content.WriteString(sectionStyle.Render("Summary:"))
	content.WriteString("\n")
	if len(c.audit.DuplicateServers) == 0 && len(c.audit.Warnings) == 0 {
		content.WriteString("  ✅ No configuration issues detected\n")
	} else {
		content.WriteString(fmt.Sprintf("  ⚠️  %d duplicate servers, %d warnings\n",
			len(c.audit.DuplicateServers), len(c.audit.Warnings)))
	}

	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	help := renderDialogHelp(t, &c.help, c, innerWidth)

	fullContent := t.Dialog.View.
		Width(width).
		Height(height).
		Render(content.String() + "\n" + help)

	DrawCenter(scr, area, fullContent)
	return nil
}

func (c *ConfigAudit) pathStatus(path string) string {
	if _, err := os.Stat(path); err == nil {
		return c.com.Styles.Tool.IconSuccess.Render("✓ " + path)
	}
	return c.com.Styles.Dialog.SecondaryText.Render("✗ " + path + " (not found)")
}

// ShortHelp implements [help.KeyMap].
func (c *ConfigAudit) ShortHelp() []key.Binding {
	return []key.Binding{c.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (c *ConfigAudit) FullHelp() [][]key.Binding {
	return [][]key.Binding{{c.keyMap.Close}}
}

// RunConfigAudit performs a configuration audit.
func RunConfigAudit(globalConfigPath, projectConfigPath string, loadedMCP map[string]interface{}) ConfigAuditResult {
	result := ConfigAuditResult{
		GlobalConfigPath:  globalConfigPath,
		ProjectConfigPath: projectConfigPath,
		Warnings:          []ConfigWarning{},
		DuplicateServers:  []DuplicateServer{},
		OrphanedFiles:     []string{},
	}

	// Check for orphaned mcp.json files
	homeDir, _ := os.UserHomeDir()
	orphans := []string{
		filepath.Join(homeDir, ".floyd", "mcp.json"),
		filepath.Join(homeDir, ".claude", "mcp.json"),
		filepath.Join(homeDir, ".config", "claude-code", "mcp.json"),
	}
	for _, orphan := range orphans {
		if _, err := os.Stat(orphan); err == nil {
			result.OrphanedFiles = append(result.OrphanedFiles, orphan)
			result.Warnings = append(result.Warnings, ConfigWarning{
				Level:   "warn",
				Message: "Config file exists but is not loaded by floyd",
				Source:  filepath.Base(orphan),
			})
		}
	}

	// Check for duplicate server definitions
	_ = loadedMCP // Reserved for future use

	return result
}
