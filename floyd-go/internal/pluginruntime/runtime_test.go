package pluginruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/hooks"
	"github.com/legacy-ai/floyd/internal/oauth"
	"github.com/legacy-ai/floyd/internal/skills"
	"github.com/stretchr/testify/require"
)

func TestCanonicalPluginReloadOwnsOnlyItsContributions(t *testing.T) {
	dir := t.TempDir()
	skillRoot := filepath.Join(dir, "skills")
	skillDir := filepath.Join(skillRoot, "fixture")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	instruction := "Use this fixture's exact guidance: $(false) and 'quoted text'."
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: fixture\ndescription: Canonical plugin fixture\n---\n\n"+instruction+"\n"), 0600))
	pluginRoot := filepath.Join(dir, "plugins")
	require.NoError(t, os.MkdirAll(pluginRoot, 0755))
	ownRoot := filepath.Join(dir, "own-skills")
	require.NoError(t, os.MkdirAll(ownRoot, 0755))
	cfg := &config.Config{Options: &config.Options{SkillsPaths: []string{ownRoot}}, Hooks: map[string][]config.HookConfig{"Stop": {{Name: "owner-hook", Command: "printf owner"}}}, MCP: config.MCPs{"owner-server": {Command: "owner-command", Disabled: true}}}
	input := Input{SkillPaths: []string{skillRoot, ownRoot}, PluginPaths: []string{pluginRoot}, SystemPrompts: []SystemPrompt{{PluginID: "fixture", Content: "Enabled plugin guidance"}}, SessionStarts: []SessionStart{{PluginID: "fixture", SkillName: "fixture"}}, MCP: config.MCPs{"plugin-server": {Command: "plugin-command", Disabled: true}}}
	same, err := Unchanged(cfg, Input{})
	require.NoError(t, err)
	require.True(t, same)
	snapshot, err := Merge(cfg, input, dir, nil)
	require.NoError(t, err)
	same, err = Unchanged(cfg, input)
	require.NoError(t, err)
	require.True(t, same)
	nativeToken := &oauth.Token{AccessToken: "plugin-account-fixture"}
	server := cfg.MCP["plugin-server"]
	server.OAuthToken = nativeToken
	cfg.MCP["plugin-server"] = server
	_, err = Merge(cfg, input, dir, nil)
	require.NoError(t, err)
	require.Same(t, nativeToken, cfg.MCP["plugin-server"].OAuthToken, "refreshing plugin guidance must retain native MCP login")
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "extra.md"), []byte("new plugin content"), 0600))
	same, err = Unchanged(cfg, input)
	require.NoError(t, err)
	require.False(t, same)
	require.Equal(t, []string{skillRoot}, snapshot.OwnedSkillPaths)
	manager := skills.NewManager(nil, nil, nil, skills.WithWorkingDir(dir))
	defer manager.Shutdown()
	events := manager.SubscribeEvents(t.Context())
	manager.Reload(skills.DiscoveryConfig{SkillsPaths: cfg.Options.SkillsPaths, WorkingDir: dir})
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("existing skill subscriber missed reload")
	}
	var found bool
	for _, skill := range manager.ActiveSkills() {
		if skill.Name == "fixture" {
			found = true
		}
	}
	require.True(t, found)
	runner := hooks.NewRunner(cfg.Hooks["SessionStart"], dir, dir)
	result, err := runner.Run(t.Context(), "SessionStart", "fixture-session", "", "")
	require.NoError(t, err)
	require.Contains(t, result.Context, instruction, "session-start guidance must pass literally through native hook execution")
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	var restored config.Config
	require.NoError(t, json.Unmarshal(encoded, &restored))
	saved, err := Read(&restored)
	require.NoError(t, err)
	require.Equal(t, "Enabled plugin guidance", saved.SystemPrompts[0].Content)
	_, err = Merge(&restored, Input{}, dir, nil)
	require.NoError(t, err)
	require.Equal(t, []string{ownRoot}, restored.Options.SkillsPaths)
	require.Empty(t, restored.Options.PluginsPaths)
	require.Contains(t, restored.MCP, "owner-server")
	require.NotContains(t, restored.MCP, "plugin-server")
	require.Len(t, restored.Hooks["Stop"], 1)
	require.Equal(t, "owner-hook", restored.Hooks["Stop"][0].Name)
	for _, entries := range restored.Hooks {
		for _, hook := range entries {
			require.False(t, strings.HasPrefix(hook.Name, hookPrefix))
		}
	}
	_, err = Merge(&restored, Input{MCP: config.MCPs{"owner-server": {Command: "replacement"}}}, dir, nil)
	require.ErrorContains(t, err, "conflicts")
}

func TestApplyCompletesInsideConfigTransaction(t *testing.T) {
	t.Setenv("FLOYD_GLOBAL_CONFIG", t.TempDir())
	t.Setenv("FLOYD_GLOBAL_DATA", t.TempDir())
	store, err := config.Load(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)
	pluginRoot := filepath.Join(t.TempDir(), "plugins")
	require.NoError(t, os.MkdirAll(pluginRoot, 0755))

	done := make(chan error, 1)
	go func() {
		_, applyErr := Apply(store, Input{PluginPaths: []string{pluginRoot}})
		done <- applyErr
	}()
	select {
	case applyErr := <-done:
		require.NoError(t, applyErr)
	case <-time.After(120 * time.Second):
		buf := make([]byte, 1<<20)
		buf = buf[:runtime.Stack(buf, true)]
		t.Fatalf("Apply deadlocked reading the resolver inside the config transaction\n%s", buf)
	}
}
