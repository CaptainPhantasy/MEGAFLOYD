package config

import (
	"fmt"
	"strings"

	"github.com/legacy-ai/floyd/internal/oauth"
)

func (s *ConfigStore) SetMCPOAuthToken(name string, token *oauth.Token) error {
	if _, exists := s.Config().MCP[name]; !exists {
		return fmt.Errorf("mcp %q is not configured", name)
	}
	scope := ScopeGlobal
	if s.workspacePath != "" {
		scope = ScopeWorkspace
	}
	key := "mcp." + strings.NewReplacer(`\`, `\\`, `.`, `\.`).Replace(name) + ".oauth_token"
	return s.update(scope, func(cfg *Config) map[string]any {
		server := cfg.MCP[name]
		server.OAuthToken = token
		cfg.MCP[name] = server
		return map[string]any{key: token}
	})
}

func (s *ConfigStore) ClearMCPOAuthToken(name string) error {
	return s.SetMCPOAuthToken(name, nil)
}
