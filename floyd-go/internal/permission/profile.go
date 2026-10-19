package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProfileGrant represents a single permission rule persisted to disk.
// Rules are evaluated in order; the first match wins.
type ProfileGrant struct {
	// Tool name pattern. Supports exact match or "*" for any tool.
	ToolName string `json:"tool_name"`
	// Action pattern. Supports exact match or "*" for any action.
	Action string `json:"action"`
	// Path pattern. Supports prefix match ending with "/" for directory scope,
	// "**" glob for recursive matching, or exact path.
	Path string `json:"path,omitempty"`
	// Description of why this rule exists (documentation only).
	Description string `json:"description,omitempty"`
	// Granted is true to auto-approve, false to auto-deny.
	Granted bool `json:"granted"`
	// TTL is the duration in seconds after which this rule expires.
	// Zero means the rule never expires.
	TTL int64 `json:"ttl,omitempty"`
	// CreatedAt is the Unix timestamp when the rule was created.
	CreatedAt int64 `json:"created_at,omitempty"`
}

// PermissionProfile is the top-level structure for a persisted permission file.
type PermissionProfile struct {
	// Grants is the ordered list of permission rules.
	Grants []ProfileGrant `json:"grants"`
	// Version for future schema migrations.
	Version int `json:"version"`
}

const (
	// ProfileVersion is the current profile schema version.
	ProfileVersion = 1
	// DefaultProfileFilename is the name of the profile file looked up
	// in the profile directory.
	DefaultProfileFilename = "permissions.json"
)

// LoadProfiles reads permission profiles from the given directory.
// It looks for DefaultProfileFilename in profileDir. If profileDir
// is empty, no profiles are loaded. Missing files are not errors.
func LoadProfiles(profileDir string) ([]ProfileGrant, error) {
	if profileDir == "" {
		return nil, nil
	}

	path := filepath.Join(profileDir, DefaultProfileFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read permission profile %s: %w", path, err)
	}

	var profile PermissionProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse permission profile %s: %w", path, err)
	}

	if profile.Version > ProfileVersion {
		return nil, fmt.Errorf("permission profile version %d is newer than supported version %d", profile.Version, ProfileVersion)
	}

	// Filter out expired grants.
	now := time.Now().Unix()
	var valid []ProfileGrant
	for _, g := range profile.Grants {
		if g.TTL > 0 && g.CreatedAt > 0 {
			if now > g.CreatedAt+g.TTL {
				continue // expired
			}
		}
		valid = append(valid, g)
	}

	return valid, nil
}

// SaveProfile persists a permission profile to disk. The profileDir
// must exist; it will not be created.
func SaveProfile(profileDir string, grants []ProfileGrant) error {
	if profileDir == "" {
		return fmt.Errorf("permission profile directory is empty")
	}

	profile := PermissionProfile{
		Grants:  grants,
		Version: ProfileVersion,
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal permission profile: %w", err)
	}

	path := filepath.Join(profileDir, DefaultProfileFilename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write permission profile %s: %w", path, err)
	}

	return nil
}

// matchProfile checks if a given tool/action/path combination matches
// a profile grant rule. Returns true if there is a match and whether
// it is an explicit deny (granted=false).
func matchProfile(grant ProfileGrant, toolName, action, path string) (matched, granted bool) {
	if !matchPattern(grant.ToolName, toolName) {
		return false, false
	}
	if !matchPattern(grant.Action, action) {
		return false, false
	}
	if grant.Path != "" && path != "" {
		if !matchPathPattern(grant.Path, path) {
			return false, false
		}
	}
	return true, grant.Granted
}

// matchPattern does exact or wildcard matching.
func matchPattern(pattern, value string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	return pattern == value
}

// matchPathPattern does exact, prefix, or recursive glob matching for paths.
func matchPathPattern(pattern, path string) bool {
	if pattern == "*" {
		return true
	}
	// Exact match
	if pattern == path {
		return true
	}
	// Directory prefix match: pattern ending with "/" matches any child.
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}
	// Recursive glob: "**" matches any subpath.
	if strings.Contains(pattern, "**") {
		// Normalize: replace ** with a glob wildcard approach
		prefix := strings.TrimSuffix(pattern, "**")
		if prefix == "" || prefix == "/" || strings.HasSuffix(prefix, "/") {
			return true // /**/ matches everything
		}
		return strings.HasPrefix(path, prefix)
	}
	return false
}
