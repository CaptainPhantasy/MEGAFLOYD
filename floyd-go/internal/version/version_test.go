package version

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildIdentityAndChangelogOutput(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	require.NoError(t, err)
	stat, err := os.Stat(exe)
	require.NoError(t, err)
	require.Equal(t, strconv.FormatInt(stat.ModTime().UnixNano(), 36), deriveBuildID())
	require.NotEmpty(t, BuildID)
	raw, err := GetJSON()
	require.NoError(t, err)
	var log Changelog
	require.NoError(t, json.Unmarshal([]byte(raw), &log))
	require.Equal(t, Version, log.CurrentVersion)
	require.NotEmpty(t, log.Entries)
	require.Contains(t, GetBootSummary(), Version)
	require.Contains(t, FormatForBoot(), "MegaFloyd "+Version)
}

func TestVersionComparisonUsesNumericComponents(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		new, old string
		want     bool
	}{{"v1.10", "v1.9", true}, {"v1.9", "v1.10", false}, {"v1.8.1", "v1.8", true}, {"v1.8", "v1.8", false}, {"v1.8.0-beta", "v1.8.0", false}, {"v1.8.0", "v1.8.0-beta", true}, {"devel", "v1.0", false}} {
		require.Equal(t, tt.want, isNewerVersion(tt.new, tt.old), "%s after %s", tt.new, tt.old)
	}
}
