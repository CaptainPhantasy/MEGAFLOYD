package prompt

import (
	"path/filepath"
	"strings"
)

// policyRoot excludes drive and UNC share roots from project policy discovery.
func policyRoot(path string) bool {
	clean := filepath.Clean(path)
	return filepath.Dir(clean) == clean || strings.TrimRight(clean, `\/`) == filepath.VolumeName(clean)
}
