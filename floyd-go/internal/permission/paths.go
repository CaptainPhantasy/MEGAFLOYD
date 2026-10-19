package permission

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// PathOutsideError reports that a tool tried to touch a path outside the
// working directory and every configured additional directory.
type PathOutsideError struct {
	Path  string
	Roots []string
}

func (e *PathOutsideError) Error() string {
	return fmt.Sprintf(
		"path %q is outside the allowed workspace directories (%s); add the directory with /add-dir",
		e.Path, strings.Join(e.Roots, ", "),
	)
}

// SetAdditionalDirs replaces the extra roots file tools may access.
// Entries are made absolute against the service working directory,
// cleaned, and deduplicated. Empty entries are ignored.
func (s *permissionService) SetAdditionalDirs(dirs []string) {
	normalized := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(s.workingDir, dir)
		}
		dir = filepath.Clean(dir)
		if slices.Contains(normalized, dir) {
			continue
		}
		normalized = append(normalized, dir)
	}
	s.additionalDirsMu.Lock()
	s.additionalDirs = normalized
	s.additionalDirsMu.Unlock()
}

// AdditionalDirs returns a copy of the extra roots in effect.
func (s *permissionService) AdditionalDirs() []string {
	s.additionalDirsMu.RLock()
	defer s.additionalDirsMu.RUnlock()
	return slices.Clone(s.additionalDirs)
}

// ValidatePath reports whether path lives inside the working directory or
// any additional root. Symlinks are resolved on the deepest existing
// component, so a link inside an allowed root that points outside it is
// still rejected, and a not-yet-created file is checked against its
// existing parent.
func (s *permissionService) ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return &PathOutsideError{Path: path}
	}
	resolved, err := resolveExisting(abs)
	if err != nil {
		return &PathOutsideError{Path: abs}
	}

	roots := make([]string, 0, 1+len(s.additionalDirs))
	roots = append(roots, s.workingDir)
	roots = append(roots, s.AdditionalDirs()...)

	for _, root := range roots {
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if resolvedRoot, err := resolveExisting(absRoot); err == nil {
			absRoot = resolvedRoot
		}
		if containsPath(absRoot, resolved) {
			return nil
		}
	}

	return &PathOutsideError{Path: resolved, Roots: roots}
}

// containsPath reports whether target is root itself or a descendant of
// it, using lexical containment on already-cleaned absolute paths.
func containsPath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExisting resolves symlinks on the deepest existing ancestor of
// path and re-appends the missing suffix. This keeps a write to a new file
// inside a symlinked project directory inside that project.
func resolveExisting(path string) (string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
