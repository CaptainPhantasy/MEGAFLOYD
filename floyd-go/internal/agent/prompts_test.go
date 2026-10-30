package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/agent/prompt"
	"github.com/legacy-ai/floyd/internal/config"
)

// TestCoderPromptIgnoresLegacyPromptOverride proves an external prompt file is
// never read: coderPrompt renders the embedded bundle even when a legacy
// prompts/coder.md.tpl exists in the global config directory.
func TestCoderPromptIgnoresLegacyPromptOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", dir)
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "coder.md.tpl"), []byte("legacy external behavior"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := coderPrompt(prompt.WithWorkingDir(dir))
	if err != nil {
		t.Fatalf("build coder prompt: %v", err)
	}
	out, err := p.Build(context.Background(), "", "", config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, dir))
	if err != nil {
		t.Fatalf("render coder prompt: %v", err)
	}
	if strings.Contains(out, "legacy external behavior") {
		t.Fatal("legacy prompt override replaced the reviewed standard")
	}
	if !strings.Contains(out, prompt.StandardPolicy()) {
		t.Fatal("reviewed standard missing from the rendered prompt")
	}
}

// TestEveryRuntimeProfileUsesReviewedPolicy keeps the collapsed dispatch
// honest: every executable alias renders the single policy-bearing template
// and therefore the reviewed policy exactly once.
func TestEveryRuntimeProfileUsesReviewedPolicy(t *testing.T) {
	for _, profile := range []string{"", "megafloyd", "superfloyd", "floyd"} {
		t.Run("profile="+profile, func(t *testing.T) {
			t.Setenv("FLOYD_RUNTIME_PROFILE", profile)
			dir := t.TempDir()
			p, err := coderPrompt(prompt.WithWorkingDir(dir))
			if err != nil {
				t.Fatalf("build coder prompt: %v", err)
			}
			out, err := p.Build(context.Background(), "", "", config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, dir))
			if err != nil {
				t.Fatalf("render coder prompt: %v", err)
			}
			if got := strings.Count(out, prompt.StandardPolicy()); got != 1 {
				t.Fatalf("profile %q renders the reviewed policy %d times, want exactly 1", profile, got)
			}
		})
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
