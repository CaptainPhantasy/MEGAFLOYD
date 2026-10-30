package config

import (
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

func TestMegaFloydPolicyDefaultsExcludeOtherGovernance(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "")
	t.Setenv("MEGAFLOYD_BINARY_NAME", "")
	project := t.TempDir()
	cfg := &Config{Options: &Options{
		ContextPaths:       []string{"AGENTS.md", "FLOYD.md", "docs/mEgAfLoYd.Md", "MegaFloyd.local.md", "MegaFloyd.md.bak"},
		GlobalContextPaths: []string{"/elsewhere/AGENTS.md", "/chosen/mEgAfLoYd.Md"},
		InitializeAs:       "AGENTS.md",
	}}
	cfg.setDefaults(project, filepath.Join(project, ".megafloyd"))
	require.Equal(t, []string{"MegaFloyd.md", "docs/mEgAfLoYd.Md"}, cfg.Options.ContextPaths)
	require.Equal(t, []string{"/chosen/mEgAfLoYd.Md"}, cfg.Options.GlobalContextPaths)
	require.Equal(t, "MegaFloyd.md", cfg.Options.InitializeAs)
	cfg.setDefaults(project, filepath.Join(project, ".megafloyd"))
	require.Equal(t, []string{"MegaFloyd.md", "docs/mEgAfLoYd.Md"}, cfg.Options.ContextPaths)
}
