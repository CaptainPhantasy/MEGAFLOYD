package diffview

// hardening_test.go -- Regression tests for Issue B (Universal Deep Code Review March 2026):
//   Replace panic("unknown diffview layout") with a safe string fallback.
// White-box (package diffview) so we can set the unexported layout field.
// Run: go test ./internal/ui/diffview -run TestHardening

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHardeningDiffviewUnknownLayoutNoPanic exercises the formerly-panicking
// default branch. A zero layout value bypasses New() and hits the default case.
func TestHardeningDiffviewUnknownLayoutNoPanic(t *testing.T) {
	dv := &DiffView{
		before:   file{content: "a\n"},
		after:    file{content: "b\n"},
		layout:   0, // zero -- neither layoutUnified(1) nor layoutSplit(2)
		tabWidth: 4,
	}
	var result string
	require.NotPanics(t, func() {
		result = dv.String()
	}, "DiffView.String() must not panic on unknown layout")
	require.True(t, strings.Contains(result, "unsupported"),
		"expected fallback text to contain 'unsupported', got: %q", result)
}

// TestHardeningDiffviewValidLayoutsRender confirms valid layouts still work.
func TestHardeningDiffviewValidLayoutsRender(t *testing.T) {
	for _, tc := range []struct {
		name   string
		layout layout
	}{
		{"unified", layoutUnified},
		{"split", layoutSplit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dv := &DiffView{
				before:   file{content: "hello\n"},
				after:    file{content: "world\n"},
				layout:   tc.layout,
				tabWidth: 4,
			}
			require.NotPanics(t, func() { _ = dv.String() })
		})
	}
}

// TestHardeningNoPanicLiteralInDiffView guards against re-introduction of the panic.
func TestHardeningNoPanicLiteralInDiffView(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "diffview.go"))
	require.NoError(t, err)
	banned := "panic(\"unknown diffview layout\")"
	require.NotContains(t, string(data), banned,
		"naked panic re-introduced into diffview.go")
}
