package mcp

import (
	"fmt"
	"iter"
	"slices"
	"strings"
)

// ToPromptXML renders the currently registered MCP tools for the system
// prompt. It returns an empty string when no tools are registered.
func ToPromptXML(scope ...WorkspaceScope) string {
	return ToPromptXMLFrom(Tools(scope...))
}

// ToPromptXMLFrom renders a tool registry. It is exported so callers and
// tests can render a deterministic snapshot instead of the live registry.
func ToPromptXMLFrom(tools iter.Seq2[string, []*Tool]) string {
	servers := make(map[string][]*Tool)
	for name, list := range tools {
		if len(list) == 0 {
			continue
		}
		servers[name] = list
	}
	if len(servers) == 0 {
		return ""
	}

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	slices.Sort(names)

	var sb strings.Builder
	sb.WriteString("<available_mcp_tools>\n")
	for _, name := range names {
		tools := slices.Clone(servers[name])
		slices.SortFunc(tools, func(a, b *Tool) int {
			if a == nil {
				return 1
			}
			if b == nil {
				return -1
			}
			return strings.Compare(a.Name, b.Name)
		})

		sb.WriteString("  <server name=\"")
		sb.WriteString(escapeXML(name))
		sb.WriteString("\">\n")
		for _, tool := range tools {
			if tool == nil || tool.Name == "" {
				continue
			}
			sb.WriteString("    <tool>\n")
			fmt.Fprintf(&sb, "      <name>%s</name>\n", escapeXML(tool.Name))
			if desc := summarize(tool.Description); desc != "" {
				fmt.Fprintf(&sb, "      <description>%s</description>\n", escapeXML(desc))
			}
			sb.WriteString("    </tool>\n")
		}
		sb.WriteString("  </server>\n")
	}
	sb.WriteString("</available_mcp_tools>")
	return sb.String()
}

func summarize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const maxLen = 400
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "…"
	}
	return s
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
