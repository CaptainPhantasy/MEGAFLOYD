package prompt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/tools/mcp"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePlugin(t *testing.T, root, name, description, instructions string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + instructions + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "PLUGIN.md"), []byte(content), 0o644))
	return dir
}

func newTestPrompt(t *testing.T, tmpl, workingDir string, opts *config.Options) (*Prompt, *config.ConfigStore) {
	t.Helper()
	store := config.NewTestStore(&config.Config{Options: opts})
	p, err := NewPrompt("test", tmpl, WithWorkingDir(workingDir))
	require.NoError(t, err)
	return p, store
}

func TestPromptDataPopulatesPluginXML(t *testing.T) {
	root := t.TempDir()
	pluginDir := writePlugin(t, root, "release-guard", "Checks release readiness", "Always run the release checklist.")
	opts := &config.Options{PluginsPaths: []string{root}}
	p, store := newTestPrompt(t, "{{.AvailPluginXML}}|{{.PluginInstrXML}}", root, opts)

	data, err := p.promptData(context.Background(), "", "", store)
	require.NoError(t, err)

	assert.Contains(t, data.AvailPluginXML, `<plugin>`)
	assert.Contains(t, data.AvailPluginXML, `<name>release-guard</name>`)
	assert.Contains(t, data.AvailPluginXML, `Checks release readiness`)
	assert.Contains(t, data.AvailPluginXML, filepath.ToSlash(filepath.Join(pluginDir, "PLUGIN.md")))
	assert.Empty(t, data.PluginInstrXML)
	assert.NotContains(t, data.AvailPluginXML, `Always run the release checklist.`)
}

func TestPromptDataOmitsPluginXMLWithoutPaths(t *testing.T) {
	p, store := newTestPrompt(t, "{{if .AvailPluginXML}}{{.AvailPluginXML}}{{end}}", t.TempDir(), &config.Options{})

	data, err := p.promptData(context.Background(), "", "", store)
	require.NoError(t, err)
	assert.Empty(t, data.AvailPluginXML)
	assert.Empty(t, data.PluginInstrXML)
}

func TestPromptDataPopulatesMCPXML(t *testing.T) {
	original := mcpPromptXML
	mcpPromptXML = func(...mcp.WorkspaceScope) string {
		return "<available_mcp_tools><server name=\"fake\"/></available_mcp_tools>"
	}
	t.Cleanup(func() { mcpPromptXML = original })

	p, store := newTestPrompt(t, "<mcp>{{.AvailMCPXML}}</mcp>", t.TempDir(), &config.Options{})
	out, err := p.Build(context.Background(), "", "", store)
	require.NoError(t, err)
	assert.Contains(t, out, `<available_mcp_tools><server name="fake"/></available_mcp_tools>`)
}

func TestPromptBuildRendersPluginSections(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "alpha", "Alpha plugin", "Alpha instructions.")
	opts := &config.Options{PluginsPaths: []string{root}}
	p, store := newTestPrompt(t, "{{if .AvailPluginXML}}PLUGINS\n{{.AvailPluginXML}}\n{{end}}{{if .PluginInstrXML}}INSTRUCTIONS\n{{.PluginInstrXML}}\n{{end}}", root, opts)

	out, err := p.Build(context.Background(), "", "", store)
	require.NoError(t, err)
	assert.Contains(t, out, "PLUGINS")
	assert.Contains(t, out, "<name>alpha</name>")
	assert.NotContains(t, out, "INSTRUCTIONS")
	assert.NotContains(t, out, "Alpha instructions.")
}
