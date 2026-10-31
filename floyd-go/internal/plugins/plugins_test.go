package plugins

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverValidPluginsAndDisabledComponents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	good := filepath.Join(root, "good", PluginFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(good), 0o700))
	require.NoError(t, os.WriteFile(good, []byte("---\r\nname: sample-plugin\r\ndescription: useful tools\r\nslash_commands:\r\n  - name: inspect\r\n    trigger: /inspect\r\n    template: Review this\r\nsub_agents:\r\n  - name: reviewer\r\n    tools: [view]\r\n---\r\nRead the project.\r\n"), 0o600))
	bad := filepath.Join(root, "bad", PluginFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(bad), 0o700))
	require.NoError(t, os.WriteFile(bad, []byte("---\nname: invalid!\ndescription: broken\n---\n"), 0o600))
	found := Discover([]string{root, root})
	require.Len(t, found, 1)
	p := found[0]
	require.Equal(t, "Read the project.", p.Instructions)
	require.Equal(t, good, p.PluginFilePath)
	require.Len(t, GetSlashCommands(found), 1)
	require.Equal(t, "Review this", GetSlashCommandByTrigger(found, "/inspect").Template)
	require.Equal(t, []string{"view"}, GetSubAgent(found, "reviewer").Tools)
	p.Enabled = false
	require.Empty(t, GetSlashCommands(found))
	require.Nil(t, GetSlashCommandByTrigger(found, "/inspect"))
	require.Nil(t, GetSubAgent(found, "reviewer"))
	require.NotContains(t, ToPromptXML(found), "sample-plugin")
}

func TestPluginParsingValidationAndEscapedMetadata(t *testing.T) {
	t.Parallel()
	p := &Plugin{Name: "plugin", Description: `Tools <safe> & "quoted"`, Enabled: true, PluginFilePath: "/tmp/a&b"}
	require.NoError(t, p.Validate())
	output := ToPromptXML([]*Plugin{p})
	var decoded struct {
		Plugin struct {
			Description string `xml:"description"`
		} `xml:"plugin"`
	}
	require.NoError(t, xml.Unmarshal([]byte(output), &decoded))
	require.Equal(t, p.Description, decoded.Plugin.Description)
	p.SlashCommands = []SlashCommand{{Name: "bad", Trigger: "bad"}}
	require.ErrorContains(t, p.Validate(), "trigger must start with /")
	file := filepath.Join(t.TempDir(), PluginFileName)
	require.NoError(t, os.WriteFile(file, []byte("not frontmatter"), 0o600))
	_, err := Parse(file)
	require.ErrorContains(t, err, "frontmatter")
}
