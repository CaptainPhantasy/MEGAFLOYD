package megafloyd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Get returns the value of a canonical MEGAFLOYD_* variable, falling back to
// the FLOYD_* mirror the chassis reads. name is the suffix without prefix,
// e.g. "IDENTITY".
func Get(name string) string {
	if value := os.Getenv(EnvPrefix + name); value != "" {
		return value
	}
	return os.Getenv("FLOYD_" + name)
}

// ChassisEnv returns the FLOYD_* mirrors for d. The map always contains the
// three directory variables; identity, profile and skills are mirrored only
// when their MEGAFLOYD_* counterpart is set, so the chassis keeps its own
// defaults otherwise.
func ChassisEnv(d Dirs) map[string]string {
	env := map[string]string{
		ChassisGlobalConfigEnv: d.Config,
		ChassisGlobalDataEnv:   d.Data,
		ChassisCacheDirEnv:     d.Cache,
	}
	for _, pair := range [][2]string{
		{IdentityEnv, ChassisIdentityEnv},
		{ProfileEnv, ChassisProfileEnv},
		{SkillsEnv, ChassisSkillsDirEnv},
	} {
		if value := os.Getenv(pair[0]); value != "" {
			env[pair[1]] = value
		}
	}
	return env
}

// ApplyChassisEnv exports the FLOYD_* mirrors for d into the process
// environment. Variables that are already set are left untouched, so an
// explicit FLOYD_* override always wins. It returns the names it exported, in
// map order.
func ApplyChassisEnv(d Dirs) []string {
	var applied []string
	for name, value := range ChassisEnv(d) {
		if _, exists := os.LookupEnv(name); exists {
			continue
		}
		if err := os.Setenv(name, value); err != nil {
			slog.Warn("Failed to set megafloyd environment", "name", name, "error", err)
			continue
		}
		applied = append(applied, name)
	}
	return applied
}

// ApplyFromExecutable applies the chassis mirrors when the process was
// started from a binary named megafloyd. Other binary names (floyd, floyd-go,
// go run temporary paths) are left alone, so the fork keeps its existing
// behavior when invoked under another name.
func ApplyFromExecutable() {
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	if base != Name {
		return
	}
	d, err := Resolve()
	if err != nil {
		slog.Warn("Failed to resolve megafloyd directories", "error", err)
		return
	}
	ApplyChassisEnv(d)
}
