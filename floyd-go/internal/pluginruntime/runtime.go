// Package pluginruntime translates enabled plugins into the native Go runtime.
package pluginruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/skills"
)

const hookPrefix = "megafloyd-plugin/"

type SystemPrompt struct {
	PluginID string `json:"pluginId"`
	Content  string `json:"content"`
}
type SessionStart struct {
	PluginID  string `json:"pluginId"`
	SkillName string `json:"skillName"`
}
type Input struct {
	SkillPaths    []string                       `json:"skill_paths"`
	PluginPaths   []string                       `json:"plugin_paths"`
	AgentRoots    []string                       `json:"agent_roots"`
	SystemPrompts []SystemPrompt                 `json:"system_prompts"`
	SessionStarts []SessionStart                 `json:"session_starts"`
	Hooks         map[string][]config.HookConfig `json:"hooks"`
	MCP           config.MCPs                    `json:"mcp"`
}
type Snapshot struct {
	Digest string `json:"digest"`
	Input
	OwnedSkillPaths  []string `json:"owned_skill_paths"`
	OwnedPluginPaths []string `json:"owned_plugin_paths"`
}

func Read(cfg *config.Config) (Snapshot, error) {
	var snapshot Snapshot
	if cfg == nil || cfg.Options == nil || cfg.Options.PluginRuntime == nil {
		return snapshot, nil
	}
	data, err := json.Marshal(cfg.Options.PluginRuntime)
	if err != nil {
		return snapshot, err
	}
	err = json.Unmarshal(data, &snapshot)
	return snapshot, err
}

// Apply persists the complete plugin contribution and its ownership in one
// native workspace config transaction. It never touches provider/model fields.
func Apply(store *config.ConfigStore, in Input) (Snapshot, error) {
	var result Snapshot
	err := store.UpdatePluginRuntime(func(cfg *config.Config) (map[string]any, error) {
		var err error
		result, err = Merge(cfg, in, store.WorkingDir(), store.Resolver().ResolveValue)
		if err != nil {
			return nil, err
		}
		return map[string]any{"options.skills_paths": cfg.Options.SkillsPaths, "options.plugins_paths": cfg.Options.PluginsPaths, "options.plugin_runtime": cfg.Options.PluginRuntime, "hooks": cfg.Hooks, "mcp": cfg.MCP}, nil
	})
	return result, err
}

func Merge(cfg *config.Config, in Input, workingDir string, resolver func(string) (string, error)) (Snapshot, error) {
	prior, err := Read(cfg)
	if err != nil {
		return Snapshot{}, err
	}
	if cfg.Options == nil {
		cfg.Options = &config.Options{}
	}
	for _, group := range [][]string{in.SkillPaths, in.PluginPaths, in.AgentRoots} {
		for _, path := range group {
			if !filepath.IsAbs(path) {
				return Snapshot{}, fmt.Errorf("plugin directory must be absolute: %q", path)
			}
			stat, err := os.Stat(path)
			if err != nil {
				return Snapshot{}, err
			}
			if !stat.IsDir() {
				return Snapshot{}, fmt.Errorf("plugin path is not a directory: %s", path)
			}
		}
	}
	digest, err := fingerprint(in)
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Input: in, Digest: digest}
	cfg.Options.SkillsPaths, result.OwnedSkillPaths = mergePaths(cfg.Options.SkillsPaths, prior.OwnedSkillPaths, in.SkillPaths)
	cfg.Options.PluginsPaths, result.OwnedPluginPaths = mergePaths(cfg.Options.PluginsPaths, prior.OwnedPluginPaths, in.PluginPaths)
	hooks := make(map[string][]config.HookConfig)
	for event, entries := range cfg.Hooks {
		for _, hook := range entries {
			if !strings.HasPrefix(hook.Name, hookPrefix) {
				hooks[event] = append(hooks[event], hook)
			}
		}
	}
	for event, entries := range in.Hooks {
		for i, hook := range entries {
			hook.Name = fmt.Sprintf("%s%s/%d", hookPrefix, event, i)
			hooks[event] = append(hooks[event], hook)
		}
	}
	_, active, _ := skills.DiscoverFromConfig(skills.DiscoveryConfig{SkillsPaths: cfg.Options.SkillsPaths, DisabledSkills: cfg.Options.DisabledSkills, WorkingDir: workingDir, Resolver: resolver})
	for _, start := range in.SessionStarts {
		var instruction string
		for _, skill := range active {
			if skill.Name == start.SkillName {
				instruction = skill.Instructions
				break
			}
		}
		if instruction == "" {
			return Snapshot{}, fmt.Errorf("plugin %q session-start skill %q is unavailable", start.PluginID, start.SkillName)
		}
		body, _ := json.Marshal(map[string]any{"version": 1, "context": instruction})
		hooks["SessionStart"] = append(hooks["SessionStart"], config.HookConfig{Name: hookPrefix + "session-start/" + start.PluginID + "/" + start.SkillName, Command: "printf '%s' " + shellQuote(string(body)), Timeout: 5})
	}
	cfg.Hooks = hooks
	if err := cfg.ValidateHooks(); err != nil {
		return Snapshot{}, err
	}
	servers := maps.Clone(cfg.MCP)
	if servers == nil {
		servers = config.MCPs{}
	}
	for name := range prior.MCP {
		delete(servers, name)
	}
	for name, server := range in.MCP {
		if strings.TrimSpace(name) == "" {
			return Snapshot{}, errors.New("plugin MCP server name is empty")
		}
		if _, exists := servers[name]; exists {
			return Snapshot{}, fmt.Errorf("plugin MCP server %q conflicts with an existing non-plugin server", name)
		}
		if previous, ok := prior.MCP[name]; ok && reflect.DeepEqual(previous, server) {
			if current, exists := cfg.MCP[name]; exists {
				server.OAuthToken = current.OAuthToken
			}
		}
		servers[name] = server
	}
	cfg.MCP = servers
	encoded, err := json.Marshal(result)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot map[string]any
	if err = json.Unmarshal(encoded, &snapshot); err != nil {
		return Snapshot{}, err
	}
	cfg.Options.PluginRuntime = snapshot
	return result, nil
}
func mergePaths(current, owned, next []string) ([]string, []string) {
	out := slices.DeleteFunc(slices.Clone(current), func(p string) bool { return slices.Contains(owned, p) })
	added := []string{}
	for _, p := range next {
		if !slices.Contains(out, p) {
			out = append(out, p)
			added = append(added, p)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out, added
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func Unchanged(cfg *config.Config, in Input) (bool, error) {
	prior, err := Read(cfg)
	if err != nil {
		return false, err
	}
	if prior.Digest == "" && len(in.SkillPaths)+len(in.PluginPaths)+len(in.AgentRoots)+len(in.SystemPrompts)+len(in.SessionStarts)+len(in.Hooks)+len(in.MCP) == 0 {
		return true, nil
	}
	digest, err := fingerprint(in)
	return err == nil && digest == prior.Digest, err
}
func fingerprint(in Input) (string, error) {
	// Normalize absent and empty collections from JSON clients.
	if len(in.SkillPaths) == 0 {
		in.SkillPaths = nil
	}
	if len(in.PluginPaths) == 0 {
		in.PluginPaths = nil
	}
	if len(in.AgentRoots) == 0 {
		in.AgentRoots = nil
	}
	if len(in.SystemPrompts) == 0 {
		in.SystemPrompts = nil
	}
	if len(in.SessionStarts) == 0 {
		in.SessionStarts = nil
	}
	if len(in.Hooks) == 0 {
		in.Hooks = nil
	}
	if len(in.MCP) == 0 {
		in.MCP = nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write(data)
	roots := append(slices.Clone(in.SkillPaths), in.PluginPaths...)
	roots = append(roots, in.AgentRoots...)
	slices.Sort(roots)
	for _, root := range slices.Compact(roots) {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") || strings.Contains(entry.Name(), ".bak-") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = digest.Write([]byte(path))
			_, _ = digest.Write([]byte{0})
			_, _ = digest.Write(body)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
