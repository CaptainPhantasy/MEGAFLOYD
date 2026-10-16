package tools

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/legacy-ai/floyd/internal/log"
)

var getRg = sync.OnceValue(func() string {
	if testing.Testing() {
		return ""
	}
	path, err := exec.LookPath("rg")
	if err != nil {
		if log.Initialized() {
			slog.Warn("Ripgrep (rg) not found in $PATH. Some grep features might be limited or slower.")
		}
		return ""
	}
	return path
})

// globPruneGlobs keeps a pattern-driven walk out of large, generated or
// remotely-mounted trees. A whitelist glob (e.g. "/**/*") disables ripgrep's
// hidden-file and ignore-file filtering, so without these a search rooted at a
// home directory descends into ~/Library, toolchain caches and network mounts.
// Names mirror the post-walk skip list in fsext.SkipHidden.
var globPruneGlobs = []string{
	"!/**/Library/**",
	"!/**/Applications/**",
	"!/**/mounts/**",
	"!/**/node_modules/**",
	"!/**/vendor/**",
	"!/**/dist/**",
	"!/**/build/**",
	"!/**/__pycache__/**",
	"!/**/.git/**",
	"!/**/.rustup/**",
	"!/**/.cargo/**",
	"!/**/.cache/**",
	"!/**/.npm/**",
	"!/**/.venvs/**",
}

func getRgCmd(ctx context.Context, globPattern string) *exec.Cmd {
	name := getRg()
	if name == "" {
		return nil
	}
	// Note: we intentionally do not pass -L (follow symlinks). Following
	// symlinks lets rg escape the search root (into module caches, the nix
	// store, $HOME, etc.) and chase cycles, which pins all cores and can
	// hang. This keeps glob scoped to the tree it was pointed at, matching
	// the grep search command.
	args := []string{"--files", "--null"}
	if globPattern != "" {
		if !filepath.IsAbs(globPattern) && !strings.HasPrefix(globPattern, "/") {
			globPattern = "/" + globPattern
		}
		args = append(args, "--glob", globPattern)
		// Later globs win in ripgrep, so these re-exclusions prune the
		// traversal while leaving the pattern's matches untouched.
		for _, prune := range globPruneGlobs {
			args = append(args, "--glob", prune)
		}
	}
	return exec.CommandContext(ctx, name, args...)
}

func getRgSearchCmd(ctx context.Context, pattern, path, include string) *exec.Cmd {
	name := getRg()
	if name == "" {
		return nil
	}
	// Use -n to show line numbers, -0 for null separation to handle Windows paths
	args := []string{"--json", "-H", "-n", "-0", pattern}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, path)

	return exec.CommandContext(ctx, name, args...)
}
