package cmd

import (
	"os"
	"testing"
)

// TODO(port-wave4): the donor also tests config precedence
// (options.runtime_profile > FLOYD_RUNTIME_PROFILE > binary name); the
// chassis config has no runtime_profile option yet, so only the env and
// binary-name precedence are exercised here.

func TestProfileFromBinaryName(t *testing.T) {
	tests := map[string]runtimeProfile{
		"floyd":      runtimeProfileFloyd,
		"megafloyd":  runtimeProfileMegafloyd,
		"superfloyd": runtimeProfileSuperFloyd,
		"beast":      runtimeProfileSuperFloyd,
		"balanced":   runtimeProfileSuperFloyd,
		"safe":       runtimeProfileSuperFloyd,
		"sf":         runtimeProfileSuperFloyd,
	}

	for bin, want := range tests {
		got := profileFromBinaryName(bin)
		if got != want {
			t.Fatalf("profileFromBinaryName(%q) = %q, want %q", bin, got, want)
		}
	}
}

func TestResolveRuntimeProfilePrecedence(t *testing.T) {
	t.Run("env wins over binary", func(t *testing.T) {
		t.Setenv("FLOYD_RUNTIME_PROFILE", "superfloyd")
		got := resolveRuntimeProfile("floyd")
		if got != runtimeProfileSuperFloyd {
			t.Fatalf("got %q, want %q", got, runtimeProfileSuperFloyd)
		}
	})

	t.Run("binary fallback when env absent", func(t *testing.T) {
		_ = os.Unsetenv("FLOYD_RUNTIME_PROFILE")
		got := resolveRuntimeProfile("beast")
		if got != runtimeProfileSuperFloyd {
			t.Fatalf("got %q, want %q", got, runtimeProfileSuperFloyd)
		}
	})
}
