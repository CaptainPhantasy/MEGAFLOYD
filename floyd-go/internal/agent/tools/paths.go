package tools

import (
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
)

// checkToolPath validates a resolved path against the workspace roots
// held by the permission service. A nil service (isolated test tools and
// the agentic-fetch temp sandbox) skips the check.
func checkToolPath(permissions permission.Service, path string) error {
	if permissions == nil {
		return nil
	}
	return permissions.ValidatePath(path)
}

// deniedPathResponse turns a path validation failure into a tool error the
// model can act on.
func deniedPathResponse(err error) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse("access denied: " + err.Error())
}

// patchTargetPaths extracts the file paths named by a unified diff's ---
// and +++ headers. Deletions and creations name /dev/null on one side;
// those entries are skipped.
func patchTargetPaths(patch string) []string {
	var paths []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		var raw string
		switch {
		case strings.HasPrefix(line, "+++ "):
			raw = strings.TrimPrefix(line, "+++ ")
		case strings.HasPrefix(line, "--- "):
			raw = strings.TrimPrefix(line, "--- ")
		default:
			continue
		}
		if i := strings.IndexByte(raw, '\t'); i >= 0 {
			raw = raw[:i]
		}
		raw = strings.TrimSpace(raw)
		raw = strings.Trim(raw, "\"")
		if raw == "" || raw == "/dev/null" {
			continue
		}
		raw = strings.TrimPrefix(raw, "a/")
		raw = strings.TrimPrefix(raw, "b/")
		paths = append(paths, raw)
	}
	return paths
}

// resolvePatchPath resolves a diff path against the working directory.
func resolvePatchPath(workingDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(workingDir, path)
}
