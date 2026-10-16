package agent

import (
	"os"
	"path/filepath"
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
