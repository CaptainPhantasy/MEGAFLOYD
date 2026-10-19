// Package version provides version information and changelog for Floyd.
package version

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ChangelogEntry represents a single feature or change in the changelog.
type ChangelogEntry struct {
	Version     string   `json:"version"`
	Type        string   `json:"type"` // "feature", "improvement", "fix", "breaking"
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"` // "agent", "tools", "ui", "performance", etc.
	Date        string   `json:"date"`
	RelatedDocs []string `json:"related_docs,omitempty"`
}

// Changelog represents the version changelog.
type Changelog struct {
	CurrentVersion string           `json:"current_version"`
	LastUpdated    string           `json:"last_updated"`
	Entries        []ChangelogEntry `json:"entries"`
	Categories     []string         `json:"categories"`
}

// DefaultChangelog returns the default changelog with known V4.x features.
func DefaultChangelog() *Changelog {
	now := time.Now().Format("2006-01-02")
	return &Changelog{
		CurrentVersion: Version,
		LastUpdated:    now,
		Categories:     []string{"agent", "tools", "ui", "performance", "safety"},
		Entries: []ChangelogEntry{
			// === v1.8 ===
			{
				Version:     "v1.8",
				Type:        "fix",
				Title:       "Restored Stock Summarization Thresholds",
				Description: "Removed premature pre-flight token check that was crashing sessions at 50%. Summarization now triggers at natural ~90% threshold using remaining-token buffer logic from upstream.",
				Category:    "safety",
				Date:        "2026-03-10",
			},
			{
				Version:     "v1.8",
				Type:        "fix",
				Title:       "Skills Library Now Loads from Config Paths",
				Description: "Fixed skills library dialog to read from config.Options.SkillsPaths instead of hardcoded path. Skills from ~/.config/floyd/skills and all configured paths now appear in Ctrl+P menu.",
				Category:    "ui",
				Date:        "2026-03-10",
			},
			{
				Version:     "v1.8",
				Type:        "feature",
				Title:       "Browser Control via Floyd TTY",
				Description: "Added browser automation commands: floyd_navigate, floyd_analyze_page, floyd_type, floyd_click, floyd_ui_audit. Requires 'source /usr/local/share/floyd/floyd-tools.sh' at session start.",
				Category:    "tools",
				Date:        "2026-03-10",
			},
			{
				Version:     "v1.8",
				Type:        "feature",
				Title:       "Skill and Agent Packager Agent",
				Description: "New global agent for converting any idea/document into properly formatted Floyd skills or agents. Trigger: 'package'. Writes directly to correct disk locations.",
				Category:    "agent",
				Date:        "2026-03-10",
			},
			// === v1.7 ===
			{
				Version:     "v1.7",
				Type:        "feature",
				Title:       "CRUSH Provider Alignment",
				Description: "Aligned with Crush CLI provider settings: uses 'zai' provider ID, GLM-5 (204,800 context window) for large tasks, GLM-4.5-Air (131,072 context window) for small tasks.",
				Category:    "agent",
				Date:        "2026-03-02",
				RelatedDocs: []string{"TOKEN_COUNTING_DIFFERENCES.md", "GLM_USAGE_MODEL.md"},
			},
			{
				Version:     "v1.7",
				Type:        "feature",
				Title:       "Execution Modes (Beast/Balanced/Safe)",
				Description: "Binary aliases control parallelism: 'beast' (16 parallel), 'balanced'/'sf' (12 parallel), 'safe' (6 parallel). Copy binary to alias name to activate.",
				Category:    "agent",
				Date:        "2026-03-02",
			},
			{
				Version:     "v1.7",
				Type:        "feature",
				Title:       "Doctor Command",
				Description: "'superfloyd doctor' shows runtime health: quality gates, degradation controls, consistency lock, auto-stabilize status, failure records, and circuit state.",
				Category:    "tools",
				Date:        "2026-03-02",
			},
			{
				Version:     "v1.7",
				Type:        "improvement",
				Title:       "Context Status Tool",
				Description: "Context status tool shows token usage with cache-aware display. Warns when approaching summarization threshold based on remaining tokens.",
				Category:    "safety",
				Date:        "2026-03-02",
			},
			{
				Version:     "v1.7",
				Type:        "feature",
				Title:       "Safety Systems Suite",
				Description: "Four-tier safety: Quality Gates (prompt validation), Degradation Controls (circuit breakers), Consistency Lock (env fingerprinting), Auto-Stabilize (benchmark recovery).",
				Category:    "safety",
				Date:        "2026-03-02",
			},
			{
				Version:     "v1.7",
				Type:        "feature",
				Title:       "Paranoia Module",
				Description: "Runtime diagnostics module for zero-branch determinism. Validates environment consistency, detects poison pills, ensures reproducible execution.",
				Category:    "safety",
				Date:        "2026-03-02",
			},
			// === v4.7 (Planned) ===
			{
				Version:     "v4.7",
				Type:        "feature",
				Title:       "Session Auto-Export",
				Description: "Automatic transcript export at 60% context threshold. Saves to .floyd/transcripts/ with full message history, tool executions, and decisions.",
				Category:    "agent",
				Date:        "2026-02-28",
				RelatedDocs: []string{"contextsidebarfinal.md"},
			},
			{
				Version:     "v4.7",
				Type:        "feature",
				Title:       "Session Handoff System",
				Description: "HANDOFF.md auto-generated on export. New sessions detect prior session and can query archive tool for context recovery without loading full transcript.",
				Category:    "agent",
				Date:        "2026-02-28",
				RelatedDocs: []string{"contextsidebarfinal.md"},
			},
			{
				Version:     "v4.7",
				Type:        "feature",
				Title:       "Semantic Archive Tool",
				Description: "query_floyd_archive tool searches past sessions with semantic firewall - only indexes tool executions and code, not conversational text. Prevents persona drift.",
				Category:    "tools",
				Date:        "2026-02-28",
				RelatedDocs: []string{"internal/agent/tools/archive.go"},
			},
			// === v4.6.1 ===
			{
				Version:     "v4.6.1",
				Type:        "feature",
				Title:       "Sidebar Stoplight Indicator",
				Description: "Context-aware color indicator: 🟢 GREEN (0-50%), 🟡 YELLOW (51-59%), 🔴 RED (60%+). Compact format with bullet separators and warning line at critical threshold.",
				Category:    "ui",
				Date:        "2026-02-28",
				RelatedDocs: []string{"contextsidebarfinal.md"},
			},
			// === v4.6 ===
			{
				Version:     "v4.6",
				Type:        "feature",
				Title:       "Dual Token Display",
				Description: "Sidebar now shows both total tokens (raw context pressure) and effective tokens (after cache subtraction). Format: '85K total / 42K effective, 50% cached'.",
				Category:    "ui",
				Date:        "2026-02-28",
				RelatedDocs: []string{"sidebarfixes.md"},
			},
			{
				Version:     "v4.6",
				Type:        "improvement",
				Title:       "Conservative Warning Thresholds",
				Description: "Context warnings now based on TOTAL tokens (not effective), providing conservative alerts at 50% warning, 60% hard threshold.",
				Category:    "safety",
				Date:        "2026-02-28",
			},
			{
				Version:     "v4.6",
				Type:        "feature",
				Title:       "Forced MCP Tool Discovery",
				Description: "Agent now probes ALL running MCP servers at boot via stdin JSON-RPC, regardless of floyd.json config or tool schema visibility. 94+ tools across 13 servers are forced into awareness.",
				Category:    "agent",
				Date:        "2026-02-28",
				RelatedDocs: []string{"internal/agent/templates/floyd_protocol.md.tpl"},
			},
			{
				Version:     "v4.6",
				Type:        "improvement",
				Title:       "MCP Section Removed from Sidebar",
				Description: "Sidebar no longer displays MCP section - tools are auto-discovered at boot, making the display redundant and saving vertical space.",
				Category:    "ui",
				Date:        "2026-02-28",
			},
			{
				Version:     "v4.6",
				Type:        "improvement",
				Title:       "Telemetry Stubbed",
				Description: "All telemetry (PostHog) functions converted to no-ops for complete independence from upstream. No data collection.",
				Category:    "safety",
				Date:        "2026-02-28",
			},
			{
				Version:     "v4.6",
				Type:        "fix",
				Title:       "Prompt Quota Timestamp Fix",
				Description: "Fixed timestamp comparison bug where prompt quota never updated. Database stores seconds, comparison was using milliseconds.",
				Category:    "fix",
				Date:        "2026-02-28",
			},
			{
				Version:     "v4.6",
				Type:        "fix",
				Title:       "DB Migration Order Fix",
				Description: "Fixed ensureColumns() running before migrations, causing 'column not found' errors on fresh databases. Now runs after goose.Up().",
				Category:    "fix",
				Date:        "2026-02-28",
			},
			// === v4.0.0 - v4.5 ===
			{
				Version:     "v4.5",
				Type:        "improvement",
				Title:       "Context Caching Display",
				Description: "Added CacheReadTokens tracking and display. Shows what percentage of context was served from cache.",
				Category:    "ui",
				Date:        "2026-02-27",
			},
			{
				Version:     "v4.5",
				Type:        "improvement",
				Title:       "Error Preservation in Prompt Usage",
				Description: "Prompt usage loader now preserves existing data on database errors instead of wiping to empty.",
				Category:    "fix",
				Date:        "2026-02-27",
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Tool Registry Boot Discovery",
				Description: "Agent now discovers all available MCP tools (105+) at startup via BootToolRegistry(). Tools are categorized and searchable.",
				Category:    "agent",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/registry.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Environment State Caching",
				Description: "Floyd ecosystem components (CLI, Desktop, Harness, Chrome Extension, Mobile) are automatically discovered at boot. Agent knows all component paths without guidance.",
				Category:    "agent",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/environment/environment.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Parallel Bash Execution",
				Description: "New ParallelBashTool enables concurrent execution of independent shell commands. Max 4 concurrent jobs with automatic dependency resolution. 87% speedup on multi-command tasks.",
				Category:    "tools",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/parallel_bash.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Streaming Tool Progress",
				Description: "Long-running operations now show real-time progress updates. See stdout/stderr streaming during npm install, tests, etc.",
				Category:    "ui",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/progress.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Workflow Engine with Checkpoints",
				Description: "Agentic workflow engine with automatic checkpoint creation and rollback on failure. Complex multi-step tasks with recovery.",
				Category:    "agent",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/workflow.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "SafeOps Semantic Impact Analysis",
				Description: "Impact analysis now uses Context-Singularity for semantic dependency tracing. Risk scores (0-100) based on actual dependents, not just file counts.",
				Category:    "safety",
				Date:        "2026-02-22",
				RelatedDocs: []string{"SafeOps integration"},
			},
			{
				Version:     "v4.0.0",
				Type:        "improvement",
				Title:       "Smart Context Compression",
				Description: "Context is now intelligently compressed: preserve (system prompt, requirements), compress (tool results, exploration), discard (duplicates, failed branches). 2x effective context window.",
				Category:    "performance",
				Date:        "2026-02-22",
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Symbol Index for Code Search",
				Description: "Fast semantic code search without embeddings using tree-sitter. Search by symbol name across Go, TypeScript, Python codebases.",
				Category:    "tools",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/symbol_index.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Multi-Agent Headless Workspace",
				Description: "Workspace abstraction for multi-agent execution. Chalkboard for shared state (SUPERCACHE-backed). Multiple workers with goroutine-based isolation.",
				Category:    "agent",
				Date:        "2026-02-22",
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "Vision/Multi-modal Input Pipeline",
				Description: "Support for analyzing screenshots, diagrams, and design mockups. Vision model integration for 'What's wrong with this UI?' type queries.",
				Category:    "agent",
				Date:        "2026-02-22",
				RelatedDocs: []string{"internal/agent/tools/vision.go"},
			},
			{
				Version:     "v4.0.0",
				Type:        "improvement",
				Title:       "MCP Health Monitoring + Auto-Restart",
				Description: "MCP servers now have health monitoring with 30s interval checks. Auto-restart with exponential backoff (1s, 2s, 4s, 8s, max 60s). Max 5 attempts before marking failed.",
				Category:    "performance",
				Date:        "2026-02-22",
			},
			{
				Version:     "v4.0.0",
				Type:        "feature",
				Title:       "SUPERCACHE Namespace Support",
				Description: "SUPERCACHE now supports namespaces for project isolation. Default namespace='global' (backwards compatible). Prevents key collisions between projects.",
				Category:    "performance",
				Date:        "2026-02-22",
			},
		},
	}
}

// GetChangelog returns the changelog, loading from config if available.
func GetChangelog() *Changelog {
	changelog := DefaultChangelog()

	// Check if changelog is defined in config
	// TODO: Load from floyd.json or CLAUDE.md if defined

	return changelog
}

// GetNewFeaturesSince returns features added since the specified version.
func GetNewFeaturesSince(sinceVersion string) []ChangelogEntry {
	changelog := GetChangelog()
	var newFeatures []ChangelogEntry

	for _, entry := range changelog.Entries {
		if isNewerVersion(entry.Version, sinceVersion) {
			newFeatures = append(newFeatures, entry)
		}
	}

	return newFeatures
}

// isNewerVersion compares two version strings (simplified).
func isNewerVersion(current, previous string) bool {
	// Simplified version comparison
	// In production, use a proper semver library
	return strings.HasPrefix(current, "v") && current > previous
}

// FormatForBoot formats the changelog for display during agent boot.
func FormatForBoot() string {
	changelog := GetChangelog()

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Floyd %s - New Features:\n", changelog.CurrentVersion))

	// Group by category
	categories := make(map[string][]ChangelogEntry)
	for _, entry := range changelog.Entries {
		categories[entry.Category] = append(categories[entry.Category], entry)
	}

	// Display in priority order
	priority := []string{"agent", "tools", "performance", "safety", "ui"}
	for _, cat := range priority {
		if entries, ok := categories[cat]; ok {
			summary.WriteString(fmt.Sprintf("\n  [%s]\n", strings.Title(cat)))
			for _, entry := range entries {
				indicator := "✓"
				if entry.Type == "feature" {
					indicator = "★"
				}
				summary.WriteString(fmt.Sprintf("    %s %s: %s\n", indicator, entry.Title, entry.Description))
			}
		}
	}

	return summary.String()
}

// FormatCompact returns a compact one-line summary of new features.
func FormatCompact() string {
	changelog := GetChangelog()
	featureCount := 0
	improvementCount := 0

	for _, entry := range changelog.Entries {
		if entry.Type == "feature" {
			featureCount++
		} else if entry.Type == "improvement" {
			improvementCount++
		}
	}

	return fmt.Sprintf("%d features, %d improvements", featureCount, improvementCount)
}

// GetBootSummary returns the boot summary for changelog.
func GetBootSummary() string {
	changelog := GetChangelog()
	return fmt.Sprintf("Changelog: %s - %s", changelog.CurrentVersion, FormatCompact())
}

// GetJSON returns the changelog as JSON.
func GetJSON() (string, error) {
	changelog := GetChangelog()
	data, err := json.MarshalIndent(changelog, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal changelog: %w", err)
	}
	return string(data), nil
}

// LogBoot logs the changelog at boot time.
func LogBoot() {
	slog.Info("Floyd changelog", "version", Version, "features", FormatCompact())
}
