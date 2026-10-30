//go:build !windows

package prompt

import (
	"os"
	"path/filepath"
	"syscall"
)

// policyRoot identifies filesystem and mounted-volume roots by device identity.
func policyRoot(path string) bool {
	parent := filepath.Dir(path)
	if parent == path {
		return true
	}
	current, err := os.Stat(path)
	if err != nil {
		return false
	}
	above, err := os.Stat(parent)
	if err != nil {
		return false
	}
	here, ok := current.Sys().(*syscall.Stat_t)
	upper, parentOK := above.Sys().(*syscall.Stat_t)
	return ok && parentOK && here.Dev != upper.Dev
}
