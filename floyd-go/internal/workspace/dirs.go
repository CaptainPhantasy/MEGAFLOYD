package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/legacy-ai/floyd/internal/fsext"
)

// NormalizeAdditionalDir expands and validates a directory passed to
// /add-dir. Relative paths resolve against the workspace working
// directory; the result must exist and be a directory.
func NormalizeAdditionalDir(workDir, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("a directory path is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	expanded, err := fsext.Expand(path)
	if err != nil {
		return "", fmt.Errorf("cannot expand %s: %w", path, err)
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(workDir, expanded)
	}
	expanded = filepath.Clean(expanded)

	info, err := os.Stat(expanded)
	if err != nil {
		return "", fmt.Errorf("cannot add %s: %w", expanded, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", expanded)
	}
	return expanded, nil
}

// mergeAdditionalDir appends dir to dirs unless it is already present.
func mergeAdditionalDir(dirs []string, dir string) []string {
	if slices.Contains(dirs, dir) {
		return slices.Clone(dirs)
	}
	return append(slices.Clone(dirs), dir)
}
