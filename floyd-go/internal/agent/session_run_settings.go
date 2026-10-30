package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/fantasy"
)

var ErrInvalidSessionSettings = errors.New("invalid session agent settings")

// SessionRunSettings is a per-call snapshot. Nil allowlists inherit the installed
// palette; an explicitly empty list permits none. It never changes model config.
type SessionRunSettings struct {
	SystemPrompt  string    `json:"system_prompt,omitempty"`
	Tools         *[]string `json:"tools,omitempty"`
	MCPServers    *[]string `json:"mcp_servers,omitempty"`
	Profile       string    `json:"profile,omitempty"`
	DisabledTools []string  `json:"disabled_tools,omitempty"`
	profilePrompt string
	profileTools  *[]string
}

func ReadSessionRunSettings(metadata map[string]any) (SessionRunSettings, error) {
	var result SessionRunSettings
	raw, ok := metadata["agent_config"]
	if !ok || raw == nil {
		return result, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrInvalidSessionSettings, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("%w: %v", ErrInvalidSessionSettings, err)
	}
	return result, result.validate()
}

func PatchSessionRunSettings(metadata map[string]any, patch map[string]json.RawMessage) (SessionRunSettings, error) {
	current, err := ReadSessionRunSettings(metadata)
	if err != nil {
		return current, err
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return current, err
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return current, err
	}
	for name, value := range patch {
		switch name {
		case "system_prompt", "tools", "mcp_servers", "profile", "disabled_tools":
		default:
			return current, fmt.Errorf("%w: unknown field %q", ErrInvalidSessionSettings, name)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(fields, name)
		} else {
			fields[name] = value
		}
	}
	encoded, err = json.Marshal(fields)
	if err != nil {
		return current, err
	}
	var result SessionRunSettings
	if err := json.Unmarshal(encoded, &result); err != nil {
		return result, fmt.Errorf("%w: %v", ErrInvalidSessionSettings, err)
	}
	if err := result.validate(); err != nil {
		return result, err
	}
	var stored any
	if err := json.Unmarshal(encoded, &stored); err != nil {
		return result, err
	}
	metadata["agent_config"] = stored
	return result, nil
}

func (s SessionRunSettings) validate() error {
	groups := []struct {
		name   string
		values *[]string
	}{{"tools", s.Tools}, {"mcp_servers", s.MCPServers}, {"disabled_tools", &s.DisabledTools}}
	for _, group := range groups {
		if group.values == nil {
			continue
		}
		for _, name := range *group.values {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%w: %s contains an empty name", ErrInvalidSessionSettings, group.name)
			}
		}
	}
	return nil
}

func (s SessionRunSettings) clone() SessionRunSettings {
	clone := func(values *[]string) *[]string {
		if values == nil {
			return nil
		}
		result := slices.Clone(*values)
		if result == nil {
			result = []string{}
		}
		return &result
	}
	s.Tools = clone(s.Tools)
	s.MCPServers = clone(s.MCPServers)
	s.profileTools = clone(s.profileTools)
	s.DisabledTools = slices.Clone(s.DisabledTools)
	return s
}

func (s SessionRunSettings) promptAddition() string {
	var parts []string
	for _, part := range []string{s.profilePrompt, s.SystemPrompt} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (s SessionRunSettings) filterTools(palette []fantasy.AgentTool) []fantasy.AgentTool {
	result := make([]fantasy.AgentTool, 0, len(palette))
	for _, tool := range palette {
		if deferred, ok := tool.(*deferredMCPTool); ok {
			if selected := deferred.forSettings(s); selected != nil {
				result = append(result, selected)
			}
			continue
		}
		name := tool.Info().Name
		if s.Tools != nil && !slices.Contains(*s.Tools, name) {
			continue
		}
		if s.profileTools != nil && !slices.Contains(*s.profileTools, name) {
			continue
		}
		if slices.Contains(s.DisabledTools, name) {
			continue
		}
		if mcp, ok := tool.(*installedMCPTool); ok && s.MCPServers != nil && !slices.Contains(*s.MCPServers, mcp.server) {
			continue
		}
		result = append(result, tool)
	}
	return result
}

func (c *coordinator) resolveSessionProfile(settings SessionRunSettings) (SessionRunSettings, error) {
	if settings.Profile == "" {
		return settings, nil
	}
	profiles, err := c.RuntimeProfiles()
	if err != nil {
		return settings, err
	}
	for _, profile := range profiles {
		if profile.ID != settings.Profile {
			continue
		}
		if profile.Disabled {
			return settings, fmt.Errorf("%w: profile %q is disabled", ErrInvalidSessionSettings, settings.Profile)
		}
		settings.profilePrompt = profile.SystemPrompt
		if profile.AllowedTools != nil {
			tools := slices.Clone(profile.AllowedTools)
			settings.profileTools = &tools
		}
		return settings, nil
	}
	return settings, fmt.Errorf("%w: profile %q not found", ErrInvalidSessionSettings, settings.Profile)
}

func (c *coordinator) ValidateSessionRunSettings(settings SessionRunSettings) error {
	_, err := c.resolveSessionProfile(settings)
	return err
}

func (c *coordinator) captureSessionRunSettings(ctx context.Context, sessionID string) (SessionRunSettings, error) {
	stored, err := c.sessions.Get(ctx, sessionID)
	if err != nil {
		return SessionRunSettings{}, err
	}
	settings, err := ReadSessionRunSettings(stored.Metadata)
	if err != nil {
		return settings, err
	}
	settings, err = c.resolveSessionProfile(settings)
	if err != nil {
		return settings, err
	}
	return settings.clone(), nil
}

// sessionSettingsSnapshot reuses acceptance-time settings for queued calls.
func (c *coordinator) sessionSettingsSnapshot(ctx context.Context, accepted *AcceptedRun, sessionID string) (SessionRunSettings, error) {
	if accepted != nil && accepted.runSettings != nil {
		return accepted.runSettings.clone(), accepted.settingsErr
	}
	return c.captureSessionRunSettings(ctx, sessionID)
}
