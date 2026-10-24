package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalProviderMetadataCacheIsPrivateAcrossHomes(t *testing.T) {
	root := t.TempDir()
	firstHome := filepath.Join(root, "account-a")
	secondHome := filepath.Join(root, "account-b")
	inherited := filepath.Join(root, "old-app-data")
	legacy := filepath.Join(inherited, "floyd")
	require.NoError(t, os.MkdirAll(legacy, 0o700))
	marker := []byte("old app catalog must remain untouched")
	for _, name := range []string{"providers", "hyper"} {
		require.NoError(t, os.WriteFile(filepath.Join(legacy, name+".json"), marker, 0o600))
	}
	t.Setenv("MEGAFLOYD_ACTIVE", "1")
	t.Setenv("MEGAFLOYD_HOME", "")
	t.Setenv("MEGAFLOYD_CACHE_DIR", "")
	t.Setenv("XDG_DATA_HOME", inherited)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "old-app-cache"))
	t.Setenv("FLOYD_CACHE_DIR", filepath.Join(root, "old-floyd-cache"))

	checks := []struct{ name, home, megaHome, override, expected string }{
		{"first account default", firstHome, "", "", filepath.Join(firstHome, ".megafloyd", "cache")},
		{"second account default", secondHome, "", "", filepath.Join(secondHome, ".megafloyd", "cache")},
		{"explicit Mega home", secondHome, filepath.Join(root, "chosen-mega-home"), "", filepath.Join(root, "chosen-mega-home", "cache")},
		{"explicit Mega cache", secondHome, filepath.Join(root, "chosen-mega-home"), filepath.Join(root, "chosen-mega-cache"), filepath.Join(root, "chosen-mega-cache")},
	}
	var firstCatalog []byte
	for index, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			t.Setenv("HOME", check.home)
			t.Setenv("USERPROFILE", check.home)
			t.Setenv("MEGAFLOYD_HOME", check.megaHome)
			t.Setenv("MEGAFLOYD_CACHE_DIR", check.override)
			for _, name := range []string{"providers", "hyper"} {
				require.Equal(t, filepath.Join(check.expected, name+".json"), cachePathFor(name))
			}
			// Exercise the real cache writers without a network or model call.
			require.NoError(t, UpdateProviders("embedded"))
			require.NoError(t, UpdateHyper("embedded"))
			for _, name := range []string{"providers", "hyper"} {
				catalog, err := os.ReadFile(filepath.Join(check.expected, name+".json"))
				require.NoError(t, err)
				require.NotEmpty(t, catalog)
				old, err := os.ReadFile(filepath.Join(legacy, name+".json"))
				require.NoError(t, err)
				require.Equal(t, marker, old)
			}
			firstPath := filepath.Join(firstHome, ".megafloyd", "cache", "providers.json")
			currentFirst, err := os.ReadFile(firstPath)
			require.NoError(t, err)
			if index == 0 {
				firstCatalog = currentFirst
				_, err := os.Stat(filepath.Join(secondHome, ".megafloyd"))
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.Equal(t, firstCatalog, currentFirst)
			}
		})
	}
}
