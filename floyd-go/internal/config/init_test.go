package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// initTestStore builds a store whose data directory holds no init flag yet.
func initTestStore(t *testing.T, project string) *ConfigStore {
	t.Helper()
	return &ConfigStore{
		config: &Config{Options: &Options{
			DataDirectory: filepath.Join(t.TempDir(), "data"),
		}},
		workingDir: project,
	}
}

// TestProjectNeedsInitializationPolicyFile covers the file the initializer
// writes: a project holding only MegaFloyd.md is already initialized, under
// any capitalization of that exact name.
func TestProjectNeedsInitializationPolicyFile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"MegaFloyd.md", "megafloyd.md", "MEGAFLOYD.MD", "mEgAfLoYd.Md"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(project, name), []byte("# policy"), 0o644))

			needs, err := ProjectNeedsInitialization(initTestStore(t, project))
			require.NoError(t, err)
			require.False(t, needs)
		})
	}
}

// TestProjectNeedsInitializationWithoutPolicyFile covers projects that hold
// other files but no initialized policy. Such a project still needs
// initialization, as do near-miss filenames, since only the exact name counts.
// A directory with no visible files at all is skipped by a separate rule.
func TestProjectNeedsInitializationWithoutPolicyFile(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"main.go", "README.md", "MegaFloyd.local.md", "MegaFloyd.md.bak"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(project, name), []byte("content"), 0o644))

			needs, err := ProjectNeedsInitialization(initTestStore(t, project))
			require.NoError(t, err)
			require.True(t, needs)
		})
	}
}
