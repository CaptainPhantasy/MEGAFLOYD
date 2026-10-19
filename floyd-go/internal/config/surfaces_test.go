package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The defaults must describe the behavior that existed before the options
// were configurable: no hard command cap, a 30000 output limit, file
// mutations allowed, and a 10 MiB read cap.
func TestSurfaceDefaultsMatchExistingBehavior(t *testing.T) {
	t.Parallel()

	var nilConfig *Config
	require.Equal(t, time.Duration(0), nilConfig.CommandTimeout())
	require.Equal(t, DefaultMaxOutputBytes, nilConfig.MaxOutputBytes())
	require.True(t, nilConfig.MutatingFileOpsEnabled())
	require.Equal(t, DefaultMaxReadSizeBytes, nilConfig.MaxReadSizeBytes())

	cfg := &Config{Options: &Options{}}
	require.Equal(t, time.Duration(0), cfg.CommandTimeout())
	require.Equal(t, 30000, cfg.MaxOutputBytes())
	require.True(t, cfg.MutatingFileOpsEnabled())
	require.Equal(t, int64(10*1024*1024), cfg.MaxReadSizeBytes())
}

func TestNormalizeOptionsFillsSurfaceDefaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.NormalizeOptions()

	require.NotNil(t, cfg.Options.Execution)
	require.Equal(t, DefaultCommandTimeout, cfg.Options.Execution.CommandTimeoutSeconds)
	require.Equal(t, DefaultMaxOutputBytes, cfg.Options.Execution.MaxOutputBytes)
	require.NotNil(t, cfg.Options.FileOps)
	require.NotNil(t, cfg.Options.FileOps.Enabled)
	require.True(t, *cfg.Options.FileOps.Enabled)
	require.Equal(t, DefaultMaxReadSizeBytes, cfg.Options.FileOps.MaxReadSizeBytes)
	require.Empty(t, cfg.Options.RuntimeProfile, "an empty profile must stay empty so the binary name can decide")
}

func TestConfiguredSurfacesLoadFromJSON(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromBytes([][]byte{[]byte(`{
		"options": {
			"execution": {"command_timeout_seconds": 90, "max_output_bytes": 4096},
			"file_ops": {"enabled": false, "max_read_size_bytes": 2048},
			"runtime_profile": "superfloyd"
		}
	}`)})
	require.NoError(t, err)
	cfg.NormalizeOptions()

	require.Equal(t, 90*time.Second, cfg.CommandTimeout())
	require.Equal(t, 4096, cfg.MaxOutputBytes())
	require.False(t, cfg.MutatingFileOpsEnabled())
	require.Equal(t, int64(2048), cfg.MaxReadSizeBytes())
	require.Equal(t, string(RuntimeProfileSuperFloyd), cfg.Options.RuntimeProfile)
}

// Full load path: file discovery, merge, and setDefaults must preserve the
// new surface values.
func TestSurfacesSurviveFullConfigLoad(t *testing.T) {
	isolated := t.TempDir()
	t.Setenv("HOME", isolated)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(isolated, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(isolated, ".local", "share"))
	t.Setenv("FLOYD_GLOBAL_CONFIG", filepath.Join(isolated, ".config", "floyd"))
	t.Setenv("FLOYD_GLOBAL_DATA", filepath.Join(isolated, ".local", "share", "floyd"))
	t.Setenv(RuntimeProfileEnvVar, "")

	workDir := t.TempDir()
	dataDir := t.TempDir()
	configJSON := `{
		"options": {
			"execution": {"command_timeout_seconds": 120, "max_output_bytes": 8192},
			"file_ops": {"enabled": false, "max_read_size_bytes": 4096},
			"runtime_profile": "superfloyd"
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "floyd.json"), []byte(configJSON), 0o644))

	store, err := Load(workDir, dataDir, false)
	require.NoError(t, err)
	cfg := store.Config()

	require.Equal(t, 120*time.Second, cfg.CommandTimeout())
	require.Equal(t, 8192, cfg.MaxOutputBytes())
	require.False(t, cfg.MutatingFileOpsEnabled())
	require.Equal(t, int64(4096), cfg.MaxReadSizeBytes())
	require.Equal(t, RuntimeProfileSuperFloyd, cfg.EffectiveRuntimeProfile("floyd"))
}

func TestInvalidExecutionAndFileOpsValuesFallBackToDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromBytes([][]byte{[]byte(`{"options":{"execution":{"command_timeout_seconds":-10,"max_output_bytes":-1},"file_ops":{"max_read_size_bytes":-5}}}`)})
	require.NoError(t, err)
	cfg.NormalizeOptions()

	require.Equal(t, 0, cfg.Options.Execution.CommandTimeoutSeconds)
	require.Equal(t, DefaultMaxOutputBytes, cfg.Options.Execution.MaxOutputBytes)
	require.Equal(t, time.Duration(0), cfg.CommandTimeout())
	require.Equal(t, DefaultMaxOutputBytes, cfg.MaxOutputBytes())
	require.Equal(t, DefaultMaxReadSizeBytes, cfg.Options.FileOps.MaxReadSizeBytes)
	require.Equal(t, DefaultMaxReadSizeBytes, cfg.MaxReadSizeBytes())

	// The accessors also guard a Config built in memory that never went
	// through normalization.
	raw := &Config{Options: &Options{
		Execution: &ExecutionOptions{CommandTimeoutSeconds: -3},
		FileOps:   &FileOpsOptions{MaxReadSizeBytes: -4},
	}}
	require.Equal(t, time.Duration(0), raw.CommandTimeout())
	require.Equal(t, DefaultMaxOutputBytes, raw.MaxOutputBytes())
	require.Equal(t, DefaultMaxReadSizeBytes, raw.MaxReadSizeBytes())
}

func TestFileOpsExplicitValues(t *testing.T) {
	t.Parallel()

	off := false
	on := true
	cfg := &Config{Options: &Options{FileOps: &FileOpsOptions{
		Enabled:          &off,
		MaxReadSizeBytes: 512,
	}}}
	require.False(t, cfg.MutatingFileOpsEnabled())
	require.Equal(t, int64(512), cfg.MaxReadSizeBytes())

	cfg.Options.FileOps.Enabled = &on
	require.True(t, cfg.MutatingFileOpsEnabled())
}

func TestResolveRuntimeProfilePrecedence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		env    string
		binary string
		config string
		want   RuntimeProfile
	}{
		{"env wins over binary and config", "superfloyd", "floyd", "floyd", RuntimeProfileSuperFloyd},
		{"env floyd wins over superfloyd binary", "floyd", "beast", "superfloyd", RuntimeProfileFloyd},
		{"recognized binary wins over config", "", "beast", "floyd", RuntimeProfileSuperFloyd},
		{"generic binary falls through to config", "", "floyd", "superfloyd", RuntimeProfileSuperFloyd},
		{"absolute generic path falls through", "", "/usr/local/bin/floyd", "balanced", RuntimeProfileSuperFloyd},
		{"empty binary falls through", "", "", "superfloyd", RuntimeProfileSuperFloyd},
		{"all silent falls back to floyd", "", "floyd", "", RuntimeProfileFloyd},
		{"megafloyd binary selects superfloyd", "", "megafloyd", "floyd", RuntimeProfileSuperFloyd},
		{"windows suffix is stripped", "", "SUPERFLOYD.EXE", "floyd", RuntimeProfileSuperFloyd},
		{"invalid config value falls back to floyd", "", "floyd", "bogus", RuntimeProfileFloyd},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ResolveRuntimeProfile(tc.env, tc.binary, tc.config))
		})
	}
}

func TestEffectiveRuntimeProfileReadsEnvironment(t *testing.T) {
	t.Setenv(RuntimeProfileEnvVar, "beast")

	cfg := &Config{Options: &Options{RuntimeProfile: string(RuntimeProfileFloyd)}}
	require.Equal(t, RuntimeProfileSuperFloyd, cfg.EffectiveRuntimeProfile("floyd"))
}

func TestRuntimeProfileAliasIsCanonicalizedOnLoad(t *testing.T) {
	t.Parallel()

	cfg, err := loadFromBytes([][]byte{[]byte(`{"options":{"runtime_profile":"balanced"}}`)})
	require.NoError(t, err)
	cfg.NormalizeOptions()

	require.Equal(t, string(RuntimeProfileSuperFloyd), cfg.Options.RuntimeProfile)
}

func TestCloneForWriteCopiesSurfaceValues(t *testing.T) {
	t.Parallel()

	off := false
	cfg := &Config{Options: &Options{
		Execution: &ExecutionOptions{CommandTimeoutSeconds: 5, MaxOutputBytes: 10},
		FileOps:   &FileOpsOptions{Enabled: &off, MaxReadSizeBytes: 64},
	}}

	clone := cfg.cloneForWrite()
	require.NotSame(t, cfg.Options.Execution, clone.Options.Execution)
	require.NotSame(t, cfg.Options.FileOps, clone.Options.FileOps)
	require.NotSame(t, cfg.Options.FileOps.Enabled, clone.Options.FileOps.Enabled)

	clone.Options.Execution.MaxOutputBytes = 20
	clone.Options.FileOps.MaxReadSizeBytes = 128
	on := true
	clone.Options.FileOps.Enabled = &on

	require.Equal(t, 10, cfg.Options.Execution.MaxOutputBytes)
	require.Equal(t, int64(64), cfg.Options.FileOps.MaxReadSizeBytes)
	require.False(t, *cfg.Options.FileOps.Enabled)
}
