// Package environment provides Floyd ecosystem environment state discovery and caching.
package environment

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/home"
)

// ComponentState represents the state of a Floyd ecosystem component.
type ComponentState string

const (
	// ComponentActive means the component is installed and running.
	ComponentActive ComponentState = "active"
	// ComponentInstalled means the component is installed but not currently running.
	ComponentInstalled ComponentState = "installed"
	// ComponentConfigured means the component has configuration but may not be installed.
	ComponentConfigured ComponentState = "configured"
	// ComponentNotFound means the component cannot be found.
	ComponentNotFound ComponentState = "not_found"
)

// Component represents a Floyd ecosystem component with its state and metadata.
type Component struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // cli, desktop, harness, chrome, mobile, mcp
	Path        string         `json:"path"`
	State       ComponentState `json:"state"`
	Version     string         `json:"version,omitempty"`
	Port        int            `json:"port,omitempty"`
	Description string         `json:"description,omitempty"`
}

// EnvironmentState represents the complete Floyd ecosystem environment.
type EnvironmentState struct {
	Timestamp        string               `json:"timestamp"`
	Version          string               `json:"version"`
	WorkingDirectory string               `json:"working_directory"`
	Components       map[string]Component `json:"components"`
	GlobalToolPaths  []string             `json:"global_tool_paths"`
	MCPServers       map[string]string    `json:"mcp_servers"` // name -> path
	TotalComponents  int                  `json:"total_components"`
	ActiveComponents int                  `json:"active_components"`
}

// Discover performs environment discovery to find all Floyd components.
// The working directory is passed directly because the host config exposes
// it on the config store, not on config.Config.
func Discover(workingDir string) *EnvironmentState {
	slog.Info("Discovering Floyd ecosystem environment")

	state := &EnvironmentState{
		Timestamp:        time.Now().Format(time.RFC3339),
		Version:          "1.0",
		WorkingDirectory: workingDir,
		Components:       make(map[string]Component),
		MCPServers:       make(map[string]string),
		GlobalToolPaths:  discoverGlobalToolPaths(),
	}

	// Discover components in likely locations
	storageRoot := "/Volumes/Storage"

	// CLI (floyd-main / FloydDeployable)
	cliPath := discoverComponent(storageRoot, "floyd-main", "floyd4", "floyd")
	if cliPath != "" {
		state.Components["cli"] = Component{
			Name:        "floyd-cli",
			Type:        "cli",
			Path:        cliPath,
			State:       ComponentInstalled,
			Description: "Core CLI and agent system",
		}
	}
	// Check FloydDeployable (active development)
	deployablePath := discoverComponent(storageRoot, "floyd-sandbox/FloydDeployable", "FloydDeployable")
	if deployablePath != "" {
		state.Components["cli_development"] = Component{
			Name:        "floyd-cli-dev",
			Type:        "cli",
			Path:        deployablePath,
			State:       ComponentInstalled,
			Description: "Active CLI development branch",
		}
	}

	// Desktop (FloydDesktopWeb-v2)
	desktopPath := discoverComponent(storageRoot, "FloydDesktopWeb-v2")
	if desktopPath != "" {
		state.Components["desktop"] = Component{
			Name:        "floyd-desktop",
			Type:        "desktop",
			Path:        desktopPath,
			State:       ComponentInstalled,
			Port:        3001,
			Description: "Desktop app with full tool executor",
		}
	}

	// Harness (floyd-harness)
	harnessPath := discoverComponent(storageRoot, "floyd-harness")
	if harnessPath != "" {
		state.Components["harness"] = Component{
			Name:        "floyd-harness",
			Type:        "harness",
			Path:        harnessPath,
			State:       ComponentInstalled,
			Port:        8000,
			Description: "GLM-5 + DeepSeek API proxy",
		}
	}

	// Chrome Extension
	chromePaths := []string{
		filepath.Join(storageRoot, "FLOYD Extension for Chrome"),
		filepath.Join(storageRoot, "floyd-chrome-extension"),
		filepath.Join(storageRoot, "floyd-sandbox/FloydDeployable/floyd-chrome"),
	}
	for _, path := range chromePaths {
		if pathExists(path) {
			state.Components["chrome_extension"] = Component{
				Name:        "floyd-chrome-extension",
				Type:        "chrome",
				Path:        path,
				State:       ComponentInstalled,
				Description: "Browser control via WebSocket",
			}
			break
		}
	}

	// Mobile PWA
	mobilePaths := []string{
		filepath.Join(storageRoot, "FLOYD MOBILE  PWA w: NGROK TUNNEL"),
		filepath.Join(storageRoot, "floyd-mobile"),
		filepath.Join(storageRoot, "floydpwa"),
	}
	for _, path := range mobilePaths {
		if pathExists(path) {
			state.Components["mobile"] = Component{
				Name:        "floyd-mobile-pwa",
				Type:        "mobile",
				Path:        path,
				State:       ComponentInstalled,
				Description: "Mobile PWA for remote access",
			}
			break
		}
	}

	// MCP Servers directory
	mcpDir := filepath.Join(storageRoot, "MCP")
	if pathExists(mcpDir) {
		state.Components["mcp_directory"] = Component{
			Name:        "mcp-servers",
			Type:        "mcp",
			Path:        mcpDir,
			State:       ComponentInstalled,
			Description: "MCP server implementations",
		}
		// Discover individual MCP servers
		discoverMCPServers(state, mcpDir)
	}

	// Also check active MCP connections
	discoverActiveMCPs(state)

	// Calculate totals
	state.TotalComponents = len(state.Components)
	for _, comp := range state.Components {
		if comp.State == ComponentActive || comp.State == ComponentInstalled {
			state.ActiveComponents++
		}
	}

	slog.Info("Environment discovery complete",
		"total_components", state.TotalComponents,
		"active_components", state.ActiveComponents,
	)

	return state
}

// discoverComponent looks for a component in the given root with possible name variations.
func discoverComponent(root string, names ...string) string {
	for _, name := range names {
		// Direct path
		path := filepath.Join(root, name)
		if pathExists(path) {
			return path
		}

		// Check for main.go or package.json as indicator of valid project
		for _, indicator := range []string{"main.go", "go.mod", "package.json", "Cargo.toml"} {
			if pathExists(filepath.Join(path, indicator)) {
				return path
			}
		}
	}
	return ""
}

// pathExists checks if a path exists (file or directory).
func pathExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		// Check if it's a symbolic link
		if os.IsNotExist(err) {
			// Try to resolve symlink
			if _, linkErr := os.Lstat(path); linkErr == nil {
				// Symlink exists but target doesn't
				return true
			}
			return false
		}
		return false
	}
	return !info.IsDir() || info.IsDir()
}

// PathExists is a public helper for checking if a path exists.
func PathExists(path string) bool {
	return pathExists(path)
}

// discoverMCPServers discovers all MCP server implementations.
func discoverMCPServers(state *EnvironmentState, mcpDir string) {
	entries, err := os.ReadDir(mcpDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		serverPath := filepath.Join(mcpDir, entry.Name())
		serverName := strings.TrimSuffix(entry.Name(), "-server")

		// Check if this is a valid MCP server (has package.json or index.ts)
		isValid := false
		for _, indicator := range []string{"package.json", "index.ts", "src/index.ts"} {
			if pathExists(filepath.Join(serverPath, indicator)) {
				isValid = true
				break
			}
		}

		if isValid {
			state.MCPServers[serverName] = serverPath
		}
	}
}

// discoverActiveMCPs adds currently connected MCP servers from the runtime.
func discoverActiveMCPs(state *EnvironmentState) {
	// Get currently connected MCP servers
	mcpStates := mcp.GetStates()
	for name, info := range mcpStates {
		if info.State == mcp.StateConnected {
			// Add or update the server info
			if existingPath, ok := state.MCPServers[name]; !ok || existingPath == "" {
				// We don't have the path from discovery, but we know it's connected
				state.MCPServers[name] = "(connected)"
			}
		}
	}
}

// discoverGlobalToolPaths finds common tool installation directories.
func discoverGlobalToolPaths() []string {
	paths := []string{
		filepath.Join(home.Dir(), ".local", "bin"),
		filepath.Join(home.Dir(), ".bin"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
	}

	var found []string
	for _, path := range paths {
		if pathExists(path) {
			found = append(found, path)
		}
	}

	// Add PATH environment variable paths
	envPath := os.Getenv("PATH")
	if envPath != "" {
		for _, path := range strings.Split(envPath, string(os.PathListSeparator)) {
			if !slices.Contains(found, path) {
				found = append(found, path)
			}
		}
	}

	return found
}

// GetState retrieves the current environment state.
func GetState(workingDir string) *EnvironmentState {
	return Discover(workingDir)
}

// GetStateJSON returns the environment state as JSON.
func GetStateJSON(workingDir string) (string, error) {
	state := GetState(workingDir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal environment state: %w", err)
	}
	return string(data), nil
}

// GetComponentPath returns the path for a named component, if found.
func GetComponentPath(workingDir, name string) (string, bool) {
	state := GetState(workingDir)
	if comp, ok := state.Components[name]; ok {
		return comp.Path, true
	}
	return "", false
}

// GetComponentPaths returns a map of all component names to their paths.
func GetComponentPaths(workingDir string) map[string]string {
	state := GetState(workingDir)
	paths := make(map[string]string)
	for name, comp := range state.Components {
		if comp.Path != "" {
			paths[name] = comp.Path
		}
	}
	return paths
}

// FormatSummary returns a human-readable summary of the environment state.
func FormatSummary(state *EnvironmentState) string {
	if state == nil {
		return "Environment: unknown"
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Environment: %d components\n", state.TotalComponents))

	// Group by type
	types := make(map[string][]Component)
	for _, comp := range state.Components {
		types[comp.Type] = append(types[comp.Type], comp)
	}

	for _, typ := range []string{"cli", "desktop", "harness", "chrome", "mobile", "mcp"} {
		if comps, ok := types[typ]; ok {
			for _, comp := range comps {
				status := "✓"
				if comp.State != ComponentActive && comp.State != ComponentInstalled {
					status = "○"
				}
				summary.WriteString(fmt.Sprintf("  %s %s: %s\n", status, comp.Name, comp.Path))
			}
		}
	}

	return summary.String()
}

// BootSummary returns a one-line summary for boot logging.
func BootSummary(workingDir string) string {
	state := GetState(workingDir)
	return fmt.Sprintf("Environment: %d/%d components active", state.ActiveComponents, state.TotalComponents)
}
