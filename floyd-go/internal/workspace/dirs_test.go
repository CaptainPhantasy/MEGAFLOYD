package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAdditionalDir(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	inside := filepath.Join(work, "nested")
	require.NoError(t, os.MkdirAll(inside, 0o755))

	got, err := NormalizeAdditionalDir(work, "nested")
	require.NoError(t, err)
	require.Equal(t, inside, got)

	got, err = NormalizeAdditionalDir(work, "~")
	require.NoError(t, err)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, home, got)
}

func TestNormalizeAdditionalDirRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	file := filepath.Join(work, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := NormalizeAdditionalDir(work, "")
	require.Error(t, err)
	_, err = NormalizeAdditionalDir(work, "missing")
	require.Error(t, err)
	_, err = NormalizeAdditionalDir(work, file)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a directory")
}

func TestMergeAdditionalDirDeduplicates(t *testing.T) {
	t.Parallel()

	got := mergeAdditionalDir([]string{"/a", "/b"}, "/b")
	require.Equal(t, []string{"/a", "/b"}, got)
	got = mergeAdditionalDir([]string{"/a"}, "/c")
	require.Equal(t, []string{"/a", "/c"}, got)
}
