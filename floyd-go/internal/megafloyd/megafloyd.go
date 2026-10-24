// Package megafloyd defines the identity of the megafloyd distribution: its
// environment namespace, its home directory layout, and the FLOYD_*
// compatibility mirrors the chassis reads.
//
// megafloyd is a distinct command with its own home directory
// (~/.megafloyd). It replaces nothing: floyd, superfloyd and ff keep their
// existing homes and environment.
//
// Canonical environment variables are MEGAFLOYD_*. The chassis (a severed
// Crush fork) still reads FLOYD_*, so callers export both: the MEGAFLOYD_*
// names are authoritative, and [ChassisEnv] supplies the mirrors.
package megafloyd

const (
	// Name is the executable command name.
	Name = "megafloyd"

	// DisplayName is the product name shown to people.
	DisplayName = "MegaFloyd"

	// EnvPrefix is the canonical environment namespace.
	EnvPrefix = "MEGAFLOYD_"

	// HomeEnv overrides the megafloyd home directory.
	HomeEnv = EnvPrefix + "HOME"
	// ConfigEnv overrides the global configuration directory.
	ConfigEnv = EnvPrefix + "CONFIG_DIR"
	// DataEnv overrides the global data directory.
	DataEnv = EnvPrefix + "DATA_DIR"
	// CacheEnv overrides the cache directory.
	CacheEnv = EnvPrefix + "CACHE_DIR"
	// IdentityEnv overrides the agent identity name.
	IdentityEnv = EnvPrefix + "IDENTITY"
	// ProfileEnv enables the pprof profile endpoint when non-empty.
	ProfileEnv = EnvPrefix + "PROFILE"
	// SkillsEnv adds a global Agent Skills search directory.
	SkillsEnv = EnvPrefix + "SKILLS_DIR"
	// ActiveEnv marks a process started by the megafloyd launcher.
	ActiveEnv = EnvPrefix + "ACTIVE"

	// DefaultHomeDirName is the home directory under the user's home.
	DefaultHomeDirName = "." + Name
)

// Chassis environment variables. Each is read by the Go chassis; see
// docs/megafloyd-namespace.md at the repository root for the read sites.
const (
	ChassisGlobalConfigEnv = "FLOYD_GLOBAL_CONFIG"
	ChassisGlobalDataEnv   = "FLOYD_GLOBAL_DATA"
	ChassisCacheDirEnv     = "FLOYD_CACHE_DIR"
	ChassisIdentityEnv     = "FLOYD_IDENTITY"
	ChassisProfileEnv      = "FLOYD_PROFILE"
	ChassisSkillsDirEnv    = "FLOYD_SKILLS_DIR"
)
