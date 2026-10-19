package jobs

import (
	"path/filepath"
	"strings"
)

// Active returns the most recently created open job that declares a scope.
// A completed job is never active. When several jobs are open, the newest
// created job with a scope wins; callers use it as the contract the harness
// enforces before a write reaches the filesystem. It returns nil when no
// open job declares a scope.
func Active(dataDir string) (*Job, error) {
	list, err := List(dataDir)
	if err != nil {
		return nil, err
	}
	var active *Job
	for _, job := range list {
		if job.Status == StatusComplete || len(job.Scope) == 0 {
			continue
		}
		if active == nil || job.CreatedAt.After(active.CreatedAt) {
			active = job
		}
	}
	return active, nil
}

// ScopeAllows reports whether path resolves inside one of the declared scope
// entries. Relative paths and relative scope entries resolve against
// workingDir; absolute entries match absolute paths. A scope entry matches
// itself and everything below it, so "internal/jobs" covers
// "internal/jobs/jobs.go" but not "internal/jobsx".
func ScopeAllows(path, workingDir string, scope []string) bool {
	target := resolveScopePath(path, workingDir)
	if target == "" {
		return false
	}
	for _, allowed := range scope {
		allowedPath := resolveScopePath(allowed, workingDir)
		if allowedPath == "" {
			continue
		}
		if target == allowedPath ||
			strings.HasPrefix(target, allowedPath+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func resolveScopePath(path, workingDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}
