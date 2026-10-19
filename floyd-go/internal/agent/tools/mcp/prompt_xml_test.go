package mcp

import (
	"iter"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeRegistry(entries map[string][]*Tool) iter.Seq2[string, []*Tool] {
	return func(yield func(string, []*Tool) bool) {
		for name, tools := range entries {
			if !yield(name, tools) {
				return
			}
		}
	}
}

func TestToPromptXMLFromRendersSortedRegistry(t *testing.T) {
	t.Parallel()
	seq := fakeRegistry(map[string][]*Tool{
		"zeta": {
			{Name: "search", Description: "Search the web"},
			{Name: "fetch", Description: "Fetch <a> page & parse"},
		},
		"alpha": {
			{Name: "query", Description: "  Query\n the  database  "},
		},
		"empty": {},
	})

	out := ToPromptXMLFrom(seq)
	require.NotEmpty(t, out)

	assert.Less(t, strings.Index(out, `<server name="alpha">`), strings.Index(out, `<server name="zeta">`))
	assert.NotContains(t, out, `name="empty"`)
	assert.Contains(t, out, `<name>fetch</name>`)
	assert.Contains(t, out, `Fetch &lt;a&gt; page &amp; parse`)
	assert.Contains(t, out, `Query the database`)
	assert.Less(t, strings.Index(out, `<name>fetch</name>`), strings.Index(out, `<name>search</name>`))
	assert.True(t, strings.HasSuffix(out, "</available_mcp_tools>"))
}

func TestToPromptXMLFromEmptyRegistry(t *testing.T) {
	t.Parallel()
	assert.Empty(t, ToPromptXMLFrom(fakeRegistry(nil)))
	assert.Empty(t, ToPromptXMLFrom(fakeRegistry(map[string][]*Tool{"no-tools": nil})))
}

func TestToPromptXMLFromSkipsNilTools(t *testing.T) {
	t.Parallel()
	seq := fakeRegistry(map[string][]*Tool{
		"server": {nil, {Name: ""}, {Name: "ok", Description: "fine"}},
	})
	out := ToPromptXMLFrom(seq)
	assert.Contains(t, out, `<name>ok</name>`)
	assert.NotContains(t, out, "<name></name>")
}
