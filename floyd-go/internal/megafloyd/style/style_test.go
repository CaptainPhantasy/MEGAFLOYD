package style

import (
	"strings"
	"testing"
)

func TestApplyAppendsDirectiveOnce(t *testing.T) {
	kernel := "# Protocol Kernel v1.0\n\nBody."

	got := Apply(kernel)
	if n := strings.Count(got, Directive); n != 1 {
		t.Fatalf("directive count = %d, want 1", n)
	}
	if !strings.HasPrefix(got, kernel) {
		t.Fatalf("kernel prefix not preserved: %q", got)
	}
	if !strings.HasSuffix(got, Directive+"\n") {
		t.Fatalf("directive is not the final section: %q", got)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	once := Apply("# Protocol Kernel v1.0\n")

	twice := Apply(once)
	if twice != once {
		t.Fatalf("second Apply changed the kernel")
	}
	if n := strings.Count(twice, Directive); n != 1 {
		t.Fatalf("directive count after two applies = %d, want 1", n)
	}
}

func TestApplyEscapeHatch(t *testing.T) {
	t.Setenv(EnvVar, "off")

	kernel := "# Protocol Kernel v1.0\n"
	if got := Apply(kernel); got != kernel {
		t.Fatalf("Apply with escape hatch = %q, want %q", got, kernel)
	}
}

func TestApplyEmptyKernel(t *testing.T) {
	got := Apply("")

	if n := strings.Count(got, Directive); n != 1 {
		t.Fatalf("directive count = %d, want 1", n)
	}
}
