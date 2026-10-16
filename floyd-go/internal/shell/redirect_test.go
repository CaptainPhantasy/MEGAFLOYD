package shell

import (
	"context"
	"strings"
	"testing"
)

// TestUnsupportedRedirectOperator is a regression test for a process-killing
// panic: mvdan.cc/sh/v3 <= v3.12.1 raises "unhandled redirect op: <>" inside
// interp.Runner.redir, and that panic used to unwind out of the background
// shell goroutine and take the whole CLI down with it. Unsupported operators
// must surface as an ordinary error instead.
func TestUnsupportedRedirectOperator(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "read-write", command: "cat <> /dev/null"},
		{name: "clobber", command: "echo hi >| /dev/null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh := NewShell(&Options{WorkingDir: t.TempDir()})

			_, _, err := sh.Exec(context.Background(), tt.command)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.command)
			}
			if !strings.Contains(err.Error(), "unsupported redirect operator") {
				t.Fatalf("expected unsupported-redirect error for %q, got: %v", tt.command, err)
			}
		})
	}
}

// TestSupportedRedirectsPassValidation guards against over-rejecting: the
// operators the interpreter does support must still reach execution.
func TestSupportedRedirectsPassValidation(t *testing.T) {
	sh := NewShell(&Options{WorkingDir: t.TempDir()})

	file := "redirect_supported_test.tmp"
	t.Cleanup(func() {
		_, _, _ = sh.Exec(context.Background(), "rm -f "+file)
	})

	commands := []string{
		"echo in > " + file,
		"echo more >> " + file,
		"cat < " + file,
		"cat " + file + " > /dev/null 2>&1",
	}

	for _, command := range commands {
		if _, _, err := sh.Exec(context.Background(), command); err != nil {
			t.Fatalf("supported command %q failed: %v", command, err)
		}
	}
}
