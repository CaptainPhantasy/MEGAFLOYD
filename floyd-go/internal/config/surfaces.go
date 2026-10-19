package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Defaults for options.execution. They describe the behavior that shipped
// before these options existed, so leaving them unset changes nothing.
const (
	// DefaultCommandTimeout means no hard wall-clock cap. The bash tool
	// still moves a long command to the background after its own
	// threshold.
	DefaultCommandTimeout = 0
	// DefaultMaxOutputBytes matches the bash tool's built-in truncation
	// limit.
	DefaultMaxOutputBytes = 30000
)

// DefaultFileOpsEnabled is the default for mutating file tools. It is
// fail-open because those tools were always available before the option
// existed.
const DefaultFileOpsEnabled = true

// DefaultMaxReadSizeBytes matches the 10 MiB read cap in
// internal/fileops.NewManager.
const DefaultMaxReadSizeBytes int64 = 10 * 1024 * 1024

// ExecutionOptions bounds command execution performed by agent tools.
type ExecutionOptions struct {
	// CommandTimeoutSeconds caps a single command's wall-clock time.
	// Zero or negative means no hard cap.
	CommandTimeoutSeconds int `json:"command_timeout_seconds,omitempty" jsonschema:"description=Maximum wall-clock seconds for one command; 0 means no hard cap,default=0,example=300"`
	// MaxOutputBytes caps captured command output. Zero or negative uses
	// the built-in limit.
	MaxOutputBytes int `json:"max_output_bytes,omitempty" jsonschema:"description=Maximum captured command output in bytes; 0 uses the built-in 30000 limit,default=30000,example=30000"`
}

// CommandTimeout returns the configured wall-clock cap. Zero or negative
// values mean no cap.
func (o *ExecutionOptions) CommandTimeout() time.Duration {
	if o == nil || o.CommandTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(o.CommandTimeoutSeconds) * time.Second
}

// OutputLimit returns the configured output cap, falling back to
// DefaultMaxOutputBytes when unset or invalid.
func (o *ExecutionOptions) OutputLimit() int {
	if o == nil || o.MaxOutputBytes <= 0 {
		return DefaultMaxOutputBytes
	}
	return o.MaxOutputBytes
}

// applyDefaults replaces out-of-range values with their defaults.
func (o *ExecutionOptions) applyDefaults() {
	if o.CommandTimeoutSeconds < 0 {
		o.CommandTimeoutSeconds = DefaultCommandTimeout
	}
	if o.MaxOutputBytes <= 0 {
		o.MaxOutputBytes = DefaultMaxOutputBytes
	}
}

// FileOpsOptions controls the tools that create, modify, or delete files.
type FileOpsOptions struct {
	// Enabled permits mutating file operations. Nil and true both leave
	// them on; only an explicit false turns them off.
	Enabled *bool `json:"enabled,omitempty" jsonschema:"description=Allow tools to create\\, modify\\, or delete files,default=true"`
	// MaxReadSizeBytes caps a single file read. Zero or negative uses the
	// built-in limit.
	MaxReadSizeBytes int64 `json:"max_read_size_bytes,omitempty" jsonschema:"description=Maximum bytes a single file read may return; 0 uses the built-in 10 MiB limit,default=10485760,example=10485760"`
}

// MutationsEnabled reports whether mutating file operations are allowed.
func (o *FileOpsOptions) MutationsEnabled() bool {
	if o == nil || o.Enabled == nil {
		return DefaultFileOpsEnabled
	}
	return *o.Enabled
}

// ReadSizeLimit returns the configured read cap, falling back to
// DefaultMaxReadSizeBytes when unset or invalid.
func (o *FileOpsOptions) ReadSizeLimit() int64 {
	if o == nil || o.MaxReadSizeBytes <= 0 {
		return DefaultMaxReadSizeBytes
	}
	return o.MaxReadSizeBytes
}

// RuntimeProfileEnvVar is the environment override for the runtime
// profile. It outranks both the executable name and
// options.runtime_profile.
const RuntimeProfileEnvVar = "FLOYD_RUNTIME_PROFILE"

// RuntimeProfile identifies the behavior lane for this process.
type RuntimeProfile string

const (
	// RuntimeProfileFloyd is the plain Floyd lane.
	RuntimeProfileFloyd RuntimeProfile = "floyd"
	// RuntimeProfileSuperFloyd is the SuperFloyd resilience lane.
	RuntimeProfileSuperFloyd RuntimeProfile = "superfloyd"
)

// NormalizeRuntimeProfile maps aliases onto the two known lanes. Unknown
// non-empty values fall back to floyd, matching the existing resolver in
// internal/cmd.
func NormalizeRuntimeProfile(v string) RuntimeProfile {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "superfloyd", "beast", "balanced", "balance", "safe", "sf", "megafloyd":
		return RuntimeProfileSuperFloyd
	default:
		return RuntimeProfileFloyd
	}
}

// ProfileFromBinaryName returns the profile a clearly named executable
// selects. A generic name such as "floyd" returns the empty profile so it
// does not mask options.runtime_profile.
func ProfileFromBinaryName(binaryName string) RuntimeProfile {
	name := strings.ToLower(strings.TrimSpace(filepath.Base(strings.TrimSpace(binaryName))))
	name = strings.TrimSuffix(name, ".exe")
	if name == "" {
		return ""
	}
	switch name {
	case "superfloyd", "beast", "balanced", "balance", "safe", "sf", "megafloyd":
		return RuntimeProfileSuperFloyd
	}
	if strings.Contains(name, "superfloyd") || strings.Contains(name, "megafloyd") {
		return RuntimeProfileSuperFloyd
	}
	return ""
}

// ResolveRuntimeProfile applies the runtime profile precedence: the
// FLOYD_RUNTIME_PROFILE value, then a recognizable executable name, then
// the config value, and finally floyd.
func ResolveRuntimeProfile(envValue, binaryName, configValue string) RuntimeProfile {
	if v := strings.TrimSpace(envValue); v != "" {
		return NormalizeRuntimeProfile(v)
	}
	if profile := ProfileFromBinaryName(binaryName); profile != "" {
		return profile
	}
	if v := strings.TrimSpace(configValue); v != "" {
		return NormalizeRuntimeProfile(v)
	}
	return RuntimeProfileFloyd
}

// ExecutionSettings returns the execution limits with defaults applied.
func (o *Options) ExecutionSettings() ExecutionOptions {
	if o == nil || o.Execution == nil {
		return ExecutionOptions{MaxOutputBytes: DefaultMaxOutputBytes}
	}
	out := *o.Execution
	out.applyDefaults()
	return out
}

// FileOpSettings returns the mutating file operation settings with
// defaults applied.
func (o *Options) FileOpSettings() FileOpsOptions {
	if o == nil || o.FileOps == nil {
		enabled := DefaultFileOpsEnabled
		return FileOpsOptions{
			Enabled:          &enabled,
			MaxReadSizeBytes: DefaultMaxReadSizeBytes,
		}
	}
	out := *o.FileOps
	if out.Enabled == nil {
		enabled := DefaultFileOpsEnabled
		out.Enabled = &enabled
	}
	if out.MaxReadSizeBytes <= 0 {
		out.MaxReadSizeBytes = DefaultMaxReadSizeBytes
	}
	return out
}

// CommandTimeout returns the configured command timeout. Zero means no
// hard cap.
func (c *Config) CommandTimeout() time.Duration {
	if c == nil {
		return 0
	}
	settings := c.Options.ExecutionSettings()
	return settings.CommandTimeout()
}

// MaxOutputBytes returns the configured command output cap with its
// default applied.
func (c *Config) MaxOutputBytes() int {
	if c == nil {
		return DefaultMaxOutputBytes
	}
	settings := c.Options.ExecutionSettings()
	return settings.OutputLimit()
}

// MutatingFileOpsEnabled reports whether file-mutating tools are allowed.
func (c *Config) MutatingFileOpsEnabled() bool {
	if c == nil {
		return DefaultFileOpsEnabled
	}
	settings := c.Options.FileOpSettings()
	return settings.MutationsEnabled()
}

// MaxReadSizeBytes returns the configured single-read cap with its default
// applied.
func (c *Config) MaxReadSizeBytes() int64 {
	if c == nil {
		return DefaultMaxReadSizeBytes
	}
	settings := c.Options.FileOpSettings()
	return settings.ReadSizeLimit()
}

// EffectiveRuntimeProfile resolves the profile from the process
// environment, the executable name, and this config, in that order.
func (c *Config) EffectiveRuntimeProfile(binaryName string) RuntimeProfile {
	configValue := ""
	if c != nil && c.Options != nil {
		configValue = c.Options.RuntimeProfile
	}
	return ResolveRuntimeProfile(os.Getenv(RuntimeProfileEnvVar), binaryName, configValue)
}
