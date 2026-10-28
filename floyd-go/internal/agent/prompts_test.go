package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
)

func TestPromptTemplateUsesReviewedBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", dir)
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "coder.md.tpl"), []byte("legacy external behavior"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := promptTemplate("coder", []byte("bundled policy wrapper")); got != "bundled policy wrapper" {
		t.Fatal("legacy prompt override replaced reviewed standard")
	}
}

func TestRoleTemplateSelection(t *testing.T) {
	for _, pair := range [][2]string{{"", "megafloyd"}, {"superfloyd", "floyd"}, {"", "superfloyd"}, {"", "floyd"}} {
		got := string(roleTemplateFor(pair[0], pair[1]))
		if strings.Count(got, "{{.StandardPolicy}}") != 1 {
			t.Fatalf("alias %v does not load policy exactly once", pair)
		}
	}
}

func TestProtocolKernelContent(t *testing.T) {
	kernel := protocolKernelPrompt()
	for _, old := range []string{"ask exactly ONE", "attempt ONE fix", "NEVER summarize", "MODE SELECTOR", "ask the user to run commands"} {
		if strings.Contains(kernel, old) {
			t.Fatalf("legacy behavior retained: %s", old)
		}
	}
}

func TestFloydProtocolAvailable(t *testing.T) {
	if FloydProtocol() != prompt.StandardPolicy() {
		t.Fatal("protocol export differs from standard")
	}
}

func TestWebRoleLoadsReviewedPolicy(t *testing.T) {
	got := WebRolePrompt()
	if strings.Count(got, prompt.StandardPolicy()) != 1 {
		t.Fatal("web policy missing, altered or duplicated")
	}
	if strings.Contains(got, "{{.StandardPolicy}}") {
		t.Fatal("unrendered template leaked into web prompt")
	}
}
