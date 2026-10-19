package agent

import (
	"strings"
	"testing"

	"github.com/legacy-ai/floyd/internal/megafloyd/style"
)

func TestProtocolKernelPromptIncludesOutputStyle(t *testing.T) {
	kernel := protocolKernelPrompt()

	if n := strings.Count(kernel, style.Directive); n != 1 {
		t.Fatalf("directive count = %d, want 1", n)
	}
	if !strings.Contains(kernel, string(protocolKernelTmpl)) {
		t.Fatalf("kernel body missing from constructed prompt")
	}
}

func TestProtocolKernelPromptEscapeHatch(t *testing.T) {
	t.Setenv(style.EnvVar, "off")

	if got := protocolKernelPrompt(); got != string(protocolKernelTmpl) {
		t.Fatalf("escape hatch did not return the raw kernel")
	}
}
