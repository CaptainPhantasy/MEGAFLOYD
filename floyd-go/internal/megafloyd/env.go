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
// three directory variables and the identity mirror, which defaults to
// [DisplayName] and follows MEGAFLOYD_IDENTITY when that is set. Profile and
// skills are mirrored only when their MEGAFLOYD_* counterpart is set, so the
// chassis keeps its own defaults for them otherwise.
func ChassisEnv(d Dirs) map[string]string {
	env := map[string]string{
		ChassisGlobalConfigEnv: d.Config,
		ChassisGlobalDataEnv:   d.Data,
		ChassisCacheDirEnv:     d.Cache,
		ChassisIdentityEnv:     DisplayName,
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
// environment. The three directory mirrors and FLOYD_IDENTITY always
// overwrite an inherited value, because the MegaFloyd layout and identity
// are authoritative. FLOYD_PROFILE and FLOYD_SKILLS_DIR are written only
// when MEGAFLOYD_PROFILE or MEGAFLOYD_SKILLS_DIR is set; an inherited value
// is otherwise left untouched. Repeated calls leave the environment
// unchanged. It returns the names it exported, in no particular order.
func ApplyChassisEnv(d Dirs) []string {
	explicit := map[string]bool{
		ChassisGlobalConfigEnv: os.Getenv(ConfigEnv) != "",
		ChassisGlobalDataEnv:   os.Getenv(DataEnv) != "",
		ChassisCacheDirEnv:     os.Getenv(CacheEnv) != "",
		ChassisIdentityEnv:     os.Getenv(IdentityEnv) != "",
		ChassisProfileEnv:      os.Getenv(ProfileEnv) != "",
		ChassisSkillsDirEnv:    os.Getenv(SkillsEnv) != "",
	}
	var applied []string
	for name, value := range ChassisEnv(d) {
		pathOrIdentity := name == ChassisGlobalConfigEnv || name == ChassisGlobalDataEnv || name == ChassisCacheDirEnv || name == ChassisIdentityEnv
		if _, exists := os.LookupEnv(name); exists && !explicit[name] && !pathOrIdentity {
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
