package prompt

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

func TestMegaFloydPolicyAssemblyUsesOnlyPolicyInStableOrder(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "")
	t.Setenv("MEGAFLOYD_BINARY_NAME", "")
	project := t.TempDir()
	child := filepath.Join(project, "child")
	global := t.TempDir()
	require.NoError(t, os.Mkdir(child, 0o700))
	write := func(path, content string) { t.Helper(); require.NoError(t, os.WriteFile(path, []byte(content), 0o600)) }
	write(filepath.Join(project, "package.json"), `{}`)
	write(filepath.Join(project, "mEgAfLoYd.Md"), "ANCESTOR_POLICY")
	write(filepath.Join(child, "megafloyd.MD"), "CHILD_POLICY")
	write(filepath.Join(global, "MEGAFLOYD.md"), StandardPolicy())
	for _, name := range []string{"AGENTS.md", "FLOYD.md", "CLAUDE.md", "MegaFloyd.local.md", "MegaFloyd.md.bak"} {
		write(filepath.Join(child, name), "EXCLUDED_GOVERNANCE")
	}
	plugins := t.TempDir()
	writePlugin(t, plugins, "fixture", "TECHNICAL_CAPABILITY", "EXCLUDED_PLUGIN_BEHAVIOR")
	cfg := &config.Config{Options: &config.Options{
		ContextPaths:       []string{"AGENTS.md", "FLOYD.md", "CLAUDE.md", "MEGAFLOYD.md", "MegaFloyd.local.md", "MegaFloyd.md.bak"},
		GlobalContextPaths: []string{filepath.Join(global, "MegaFloyd.md")},
		PluginsPaths:       []string{plugins},
	}}
	store := config.NewTestStoreWithWorkingDir(cfg, child)
	role, err := os.ReadFile("../../../../docs/prompt-pack/templates/megafloyd-coder.md.tpl")
	require.NoError(t, err)
	p, err := NewPrompt("coder", string(role), WithWorkingDir(child), WithTimeFunc(func() time.Time { return time.Unix(0, 0) }))
	require.NoError(t, err)
	data, err := p.promptData(context.Background(), "fixture-provider", "fixture-model", store)
	require.NoError(t, err)
	require.Equal(t, StandardPolicy(), data.StandardPolicy)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(StandardPolicy()))), StandardPolicySHA256())
	require.Empty(t, data.GlobalContextFiles, "byte-identical installed policy must not be emitted twice")
	require.Len(t, data.ContextFiles, 2)
	require.Equal(t, "CHILD_POLICY", data.ContextFiles[0].Content)
	require.Equal(t, "ANCESTOR_POLICY", data.ContextFiles[1].Content)
	require.Empty(t, data.PluginInstrXML)
	require.Contains(t, data.AvailPluginXML, "TECHNICAL_CAPABILITY")
	first, err := p.Build(context.Background(), "fixture-provider", "fixture-model", store)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(first, StandardPolicy()))
	require.Contains(t, first, "CHILD_POLICY")
	require.Contains(t, first, "ANCESTOR_POLICY")
	require.NotContains(t, first, "EXCLUDED_GOVERNANCE")
	require.NotContains(t, first, "EXCLUDED_PLUGIN_BEHAVIOR")
	require.Less(t, strings.Index(first, StandardPolicy()), strings.Index(first, "CHILD_POLICY"))
	require.Less(t, strings.Index(first, "CHILD_POLICY"), strings.Index(first, "ANCESTOR_POLICY"))
	for i := 0; i < 12; i++ {
		again, err := p.Build(context.Background(), "fixture-provider", "fixture-model", store)
		require.NoError(t, err)
		require.Equal(t, first, again)
	}
}

func TestMegaFloydPolicyMatchesFullASCIINameAndBoundary(t *testing.T) {
	for _, name := range []string{"MegaFloyd.md", "megafloyd.md", "mEgAfLoYd.MD"} {
		require.True(t, policyName(name), name)
	}
	for _, name := range []string{"MegaFloyd.local.md", "MegaFloyd.md.bak", "MegaFlоyd.md", "AGENTS.md", "MegaFloyd"} {
		require.False(t, policyName(name), name)
	}
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	require.True(t, policyRoot(root))
	require.Empty(t, projectPolicyPaths(root))
	require.False(t, policyRoot(t.TempDir()))
	project := t.TempDir()
	nested := filepath.Join(project, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(project, "package.json"), []byte(`{}`), 0o600))
	resolved, err := filepath.EvalSymlinks(project)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(resolved, "nested", "MegaFloyd.md"), filepath.Join(resolved, "MegaFloyd.md")}, projectPolicyPaths(nested))
}

func TestMegaFloydPolicyReadFailureIsVisible(t *testing.T) {
	t.Setenv(megafloyd.ActiveEnv, "")
	t.Setenv("MEGAFLOYD_BINARY_NAME", "")
	project := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(project, "absent"), filepath.Join(project, "mEgAfLoYd.md")))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, project)
	_, err := loadContextFiles([]string{"MegaFloyd.md"}, store)
	require.ErrorContains(t, err, "reading policy")
}
