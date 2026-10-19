package jobs

import (
	"slices"
	"strings"
)

// DriftViolation is a changed path the contract did not declare in scope.
type DriftViolation struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// CheckDrift compares changed paths against the job's declared scope. An
// empty scope means no change can be shown in scope, so every change is a
// violation. The comparison is pure; callers supply the changed paths.
func CheckDrift(dataDir, jobID string, changed []string) ([]DriftViolation, error) {
	job, err := Load(dataDir, jobID)
	if err != nil {
		return nil, err
	}

	normalized := make([]string, 0, len(changed))
	for _, path := range changed {
		path = normalizePath(path)
		if path == "" || slices.Contains(normalized, path) {
			continue
		}
		normalized = append(normalized, path)
	}
	slices.Sort(normalized)

	var violations []DriftViolation
	for _, path := range normalized {
		reason := ""
		switch {
		case len(job.Scope) == 0:
			reason = "no scope declared in contract"
		case !inScope(path, job.Scope):
			reason = "outside declared scope"
		}
		if reason != "" {
			violations = append(violations, DriftViolation{Path: path, Reason: reason})
		}
	}
	return violations, nil
}

func inScope(path string, scope []string) bool {
	for _, allowed := range scope {
		allowed = normalizePath(allowed)
		if allowed == "" {
			continue
		}
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}
