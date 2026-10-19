package agent

import (
	"strings"
	"testing"
	"text/template"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
)

// TestSuperfloydRoleRenders proves the superfloyd role template executes with
// the chassis PromptDat and carries its identity, project context, and MCP
// section. This is the assembly gate for the intelligence content.
func TestSuperfloydRoleRenders(t *testing.T) {
	tmpl, err := template.New("coder").Parse(string(superfloydCoderPromptTmpl))
	if err != nil {
		t.Fatalf("parse superfloyd role template: %v", err)
	}
	data := prompt.PromptDat{
		WorkingDir:   "/tmp/project",
		IsGitRepo:    true,
		Platform:     "darwin",
		Date:         "10/19/2026",
		AvailMCPXML:  "<mcp>room</mcp>",
		ContextFiles: []prompt.ContextFile{{Path: "FLOYD.md", Content: "# Project rules"}},
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("execute superfloyd role template: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"# SUPERFLOYD", "Project Context", "FLOYD.md", "<mcp>room</mcp>", "Code Quality Gates"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered role missing %q", want)
		}
	}
}

func TestGeneralRoleRenders(t *testing.T) {
	tmpl, err := template.New("coder").Parse(string(floydGeneralPromptTmpl))
	if err != nil {
		t.Fatalf("parse general role template: %v", err)
	}
	data := prompt.PromptDat{ContextFiles: []prompt.ContextFile{{Path: "FLOYD.md", Content: "rules"}}}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("execute general role template: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"# FLOYD", "Specialization", "DETERMINISTIC MODE"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered role missing %q", want)
		}
	}
}
