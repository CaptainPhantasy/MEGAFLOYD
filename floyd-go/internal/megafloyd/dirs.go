package megafloyd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dirs is the megafloyd directory layout.
type Dirs struct {
	// Home is the root, ~/.megafloyd by default.
	Home string
	// Config holds the global floydrc/floyd.json and prompts.
	Config string
	// Data holds global data and per-invocation state (via --data-dir).
	Data string
	// Cache holds regenerable caches and server logs.
	Cache string
	// State holds credentials and other private state.
	State string
	// Logs holds launcher-visible log output.
	Logs string
}

// HomeDir returns the megafloyd home directory. MEGAFLOYD_HOME wins, then the
// default of ~/.megafloyd. A leading ~ in the override is expanded.
func HomeDir() (string, error) {
	if home := os.Getenv(HomeEnv); home != "" {
		return expandHome(home)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(userHome, DefaultHomeDirName), nil
}

// ConfigDir returns the global configuration directory. Precedence:
// MEGAFLOYD_CONFIG_DIR, then
// $MEGAFLOYD_HOME/config.
func ConfigDir(home string) string {
	if dir := os.Getenv(ConfigEnv); dir != "" {
		return mustExpand(dir)
	}
	return filepath.Join(home, "config")
}

// DataDir returns the global data directory. Precedence: MEGAFLOYD_DATA_DIR,
// then $MEGAFLOYD_HOME/data.
func DataDir(home string) string {
	if dir := os.Getenv(DataEnv); dir != "" {
		return mustExpand(dir)
	}
	return filepath.Join(home, "data")
}

// CacheDir returns the cache directory. Precedence: MEGAFLOYD_CACHE_DIR, then
// an existing FLOYD_CACHE_DIR, then $MEGAFLOYD_HOME/cache.
func CacheDir(home string) string {
	if dir := os.Getenv(CacheEnv); dir != "" {
		return mustExpand(dir)
	}
	return filepath.Join(home, "cache")
}

// Resolve returns the full directory layout for the current environment.
func Resolve() (Dirs, error) {
	home, err := HomeDir()
	if err != nil {
		return Dirs{}, err
	}
	return Dirs{
		Home:   home,
		Config: ConfigDir(home),
		Data:   DataDir(home),
		Cache:  CacheDir(home),
		State:  filepath.Join(home, "state"),
		Logs:   filepath.Join(home, "logs"),
	}, nil
}

// Ensure creates every directory in the layout.
func (d Dirs) Ensure() error {
	for _, dir := range d.All() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// All returns every directory in the layout, in display order.
func (d Dirs) All() []string {
	return []string{d.Home, d.Config, d.Data, d.Cache, d.State, d.Logs}
}

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return filepath.Clean(p), nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %q: %w", p, err)
	}
	if p == "~" {
		return userHome, nil
	}
	return filepath.Join(userHome, p[2:]), nil
}

// mustExpand expands ~ when possible and otherwise returns the cleaned path.
// The launcher has already validated writability by this point.
func mustExpand(p string) string {
	expanded, err := expandHome(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return expanded
}
