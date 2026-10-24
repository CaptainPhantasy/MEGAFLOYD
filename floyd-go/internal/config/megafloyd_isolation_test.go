package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

func TestCanonicalMegafloydConfigIsolation(t *testing.T) {
	root := t.TempDir()
	canonicalHome := filepath.Join(root, "megafloyd-home")
	project := filepath.Join(root, "project")
	parentHome := filepath.Join(root, "parent-harness")
	write := func(path, content string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	parentConfig := `{"options":{"agent_name":"parent-harness","data_directory":"parent-state"}}`
	write(filepath.Join(parentHome, "config", "floyd.json"), parentConfig)
	write(filepath.Join(parentHome, "data", "floyd.json"), parentConfig)
	write(filepath.Join(project, "floyd.json"), parentConfig)
	write(filepath.Join(project, ".floyd", "floyd.json"), parentConfig)
	write(filepath.Join(project, "floydrc"), "this parent shell config must never execute\n")
	for _, name := range []string{"FLOYD.md", "CLAUDE.md", "GEMINI.md"} {
		write(filepath.Join(project, name), "parent harness instructions")
	}
	ownConfig := filepath.Join(canonicalHome, "config", "floyd.json")
	ownDataConfig := filepath.Join(canonicalHome, "data", "floyd.json")
	ownProjectConfig := filepath.Join(project, "megafloyd.json")
	write(ownConfig, `{"options":{"agent_name":"megafloyd-global"}}`)
	write(ownDataConfig, "{}")
	write(ownProjectConfig, `{"options":{"agent_name":"megafloyd-project"}}`)
	write(filepath.Join(canonicalHome, "config", "MEGAFLOYD.md"), "canonical instructions")
	write(filepath.Join(project, "MEGAFLOYD.md"), "canonical project instructions")

	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	t.Setenv("MEGAFLOYD_HOME", canonicalHome)
	for _, key := range []string{
		"MEGAFLOYD_CONFIG_DIR", "MEGAFLOYD_DATA_DIR", "MEGAFLOYD_CACHE_DIR",
		"MEGAFLOYD_SKILLS_DIR", "MEGAFLOYD_AGENTS_DIR", "MEGAFLOYD_IDENTITY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("FLOYD_GLOBAL_CONFIG", filepath.Join(parentHome, "config"))
	t.Setenv("FLOYD_GLOBAL_DATA", filepath.Join(parentHome, "data"))
	t.Setenv("FLOYD_CACHE_DIR", filepath.Join(parentHome, "cache"))
	t.Setenv("FLOYD_SKILLS_DIR", filepath.Join(parentHome, "skills"))
	t.Setenv("FLOYD_AGENTS_DIR", filepath.Join(parentHome, "agents"))
	t.Setenv("FLOYD_IDENTITY", "parent-harness")
	realHome := os.Getenv("HOME")

	dirs, err := megafloyd.Resolve()
	require.NoError(t, err)
	megafloyd.ApplyChassisEnv(dirs)
	require.Equal(t, filepath.Join(canonicalHome, "config"), dirs.Config)
	require.Equal(t, filepath.Join(canonicalHome, "data"), dirs.Data)
	require.Equal(t, filepath.Join(canonicalHome, "cache"), GlobalCacheDir())
	require.Equal(t, realHome, os.Getenv("HOME"))

	paths := lookupConfigs(project)
	require.Contains(t, paths, ownConfig)
	require.Contains(t, paths, ownDataConfig)
	require.Contains(t, paths, ownProjectConfig)
	require.NotContains(t, paths, filepath.Join(parentHome, "config", "floyd.json"))
	require.NotContains(t, paths, filepath.Join(parentHome, "data", "floyd.json"))
	require.NotContains(t, paths, filepath.Join(project, "floyd.json"))
	require.NotContains(t, paths, filepath.Join(project, "floydrc"))
	require.NotContains(t, paths, "/etc/floyd/floyd.json")

	cfg, _, err := loadFromConfigPaths(t.Context(), paths)
	require.NoError(t, err)
	cfg.setDefaults(project, "")
	require.Equal(t, "megafloyd-project", cfg.Options.AgentName)
	require.Equal(t, filepath.Join(project, ".megafloyd"), cfg.Options.DataDirectory)
	require.ElementsMatch(t, []string{
		"AGENTS.md", "AGENTS.local.md", "MEGAFLOYD.md", "MEGAFLOYD.local.md",
	}, cfg.Options.ContextPaths)
	require.ElementsMatch(t, []string{
		filepath.Join(canonicalHome, "config", "MEGAFLOYD.md"),
		filepath.Join(canonicalHome, "AGENTS.md"),
	}, cfg.Options.GlobalContextPaths)
	require.ElementsMatch(t, []string{
		filepath.Join(canonicalHome, "config", "skills"),
		filepath.Join(project, ".megafloyd", "skills"),
	}, cfg.Options.SkillsPaths)
	require.Equal(t, []string{filepath.Join(canonicalHome, "config", "agents")}, GlobalAgentsDirs())

	t.Setenv("MEGAFLOYD_SKILLS_DIR", filepath.Join(canonicalHome, "explicit-skills"))
	require.Equal(t, []string{filepath.Join(canonicalHome, "explicit-skills")}, GlobalSkillsDirs())
}
