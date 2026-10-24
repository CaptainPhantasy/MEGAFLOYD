package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptTemplateOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", dir)

	promptsDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := "You are {{.AgentName}}, the overridden persona."
	overridePath := filepath.Join(promptsDir, "coder.md.tpl")
	if err := os.WriteFile(overridePath, []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := promptTemplate("coder", []byte("embedded default")); got != override {
		t.Fatalf("override not applied: %q", got)
	}

	if got := promptTemplate("task", []byte("embedded default")); got != "embedded default" {
		t.Fatalf("embedded fallback not used: %q", got)
	}
}

func TestRoleTemplateSelection(t *testing.T) {
	cases := []struct {
		name   string
		legacy []byte
		want   string
	}{
		{"superfloyd profile", roleTemplateFor("superfloyd", "floyd"), "# SUPERFLOYD"},
		{"megafloyd binary", roleTemplateFor("", "megafloyd"), "# MegaFloyd"},
		{"superfloyd binary", roleTemplateFor("", "superfloyd"), "# SUPERFLOYD"},
		{"general fallback", roleTemplateFor("", "floyd"), "# FLOYD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(string(tc.legacy), tc.want) {
				t.Fatalf("role template missing %q: %q", tc.want, string(tc.legacy)[:80])
			}
		})
	}
}

func TestProtocolKernelContent(t *testing.T) {
	kernel := string(protocolKernelTmpl)
	for _, want := range []string{
		"Protocol Kernel",
		"MODE SELECTOR",
		"DEBUG MODE GATES",
		"BUILD MODE GATES",
		"TOOL DISCIPLINE",
		"CONTINUITY",
	} {
		if !strings.Contains(kernel, want) {
			t.Fatalf("kernel missing %q", want)
		}
	}
}

func TestFloydProtocolAvailable(t *testing.T) {
	if strings.TrimSpace(FloydProtocol()) == "" {
		t.Fatal("FloydProtocol returned empty content")
	}
}
