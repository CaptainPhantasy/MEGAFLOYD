package fsops

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTextFileLifecyclePreservesExistingFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "nested", "source.txt")
	require.NoError(t, WriteTextFile(source, "first\nsecond\n", true, false))
	require.Error(t, WriteTextFile(source, "replace", false, false))
	require.NoError(t, AppendTextFile(source, "third", false))
	content, err := ReadTextFile(source)
	require.NoError(t, err)
	require.Equal(t, "first\nsecond\nthird", content)
	lines, err := ReadFileLines(source, 2, 99)
	require.NoError(t, err)
	require.Equal(t, []string{"second", "third"}, lines)
	lines, err = ReadFileLines(source, 9, 10)
	require.NoError(t, err)
	require.Empty(t, lines)
	destination := filepath.Join(root, "copies", "copy.txt")
	require.NoError(t, CopyFile(source, destination, false, true))
	require.Error(t, CopyFile(source, destination, false, false))
	copied, err := ReadTextFile(destination)
	require.NoError(t, err)
	require.Equal(t, content, copied)
	renamed := filepath.Join(root, "copies", "renamed.txt")
	require.NoError(t, Rename(destination, renamed))
	require.False(t, FileExists(destination))
	require.True(t, FileExists(renamed))
	require.True(t, DirectoryExists(filepath.Dir(renamed)))
	require.False(t, FileExists(filepath.Dir(renamed)))
	entries, err := ListDirectory(filepath.Dir(renamed))
	require.NoError(t, err)
	require.Equal(t, []string{"renamed.txt"}, entries)
	_, err = ReadTextFile(destination)
	require.Error(t, err)
}
