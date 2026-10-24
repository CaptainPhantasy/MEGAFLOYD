package megafloyd

import (
	"os"
	"path/filepath"
	"strings"
)

// IsActive identifies the canonical distribution even when an application
// bundle overrides the executable's display name.
func IsActive() bool {
	return strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == Name ||
		os.Getenv("MEGAFLOYD_BINARY_NAME") == Name ||
		os.Getenv(ActiveEnv) == "1"
}

// ProjectDirName separates application state from the scaffold's old files.
func ProjectDirName() string {
	if IsActive() {
		return ".megafloyd"
	}
	return ".floyd"
}
