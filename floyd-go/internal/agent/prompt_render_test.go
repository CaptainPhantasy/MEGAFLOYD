package agent

import (
	"strings"
	"testing"
	"text/template"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
)

// TestCoderRoleRenders proves the one surviving role template executes with
// the chassis PromptDat and carries its identity, the reviewed policy slot,
// project context, and the MCP section. This is the assembly gate for the
// intelligence content.
func TestCoderRoleRenders(t *testing.T) {
	tmpl, err := template.New("coder").Parse(string(megafloydCoderPromptTmpl))
	if err != nil {
		t.Fatalf("parse coder role template: %v", err)
	}
	data := prompt.PromptDat{
		AgentName:      "MegaFloyd",
		StandardPolicy: "REVIEWED POLICY BODY",
		WorkingDir:     "/tmp/project",
		IsGitRepo:      true,
		Platform:       "darwin",
		Date:           "10/19/2026",
		AvailMCPXML:    "<mcp>room</mcp>",
		ContextFiles:   []prompt.ContextFile{{Path: "FLOYD.md", Content: "# Project rules"}},
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("execute coder role template: %v", err)
	}
	out := sb.String()
	for _, want := range []string{
		"# MegaFloyd",
		"REVIEWED POLICY BODY",
		"Agent: MegaFloyd",
		"Working directory: /tmp/project",
		"<project_megafloyd_policy>",
		"Source: FLOYD.md",
		"# Project rules",
		"<mcp>room</mcp>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered role missing %q", want)
		}
	}
	for _, unwanted := range []string{"{{", "<global_megafloyd_policy>", "<skills>"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("rendered role leaked %q with empty data", unwanted)
		}
	}
}
