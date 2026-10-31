package fileops

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerWritesReadsListsAndDiffsWorkspaceFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := NewManager(Config{WorkspacePath: root, MaxReadSizeByte: 32})
	require.NoError(t, m.Initialize())
	written := m.WriteFile("nested/example.txt", "one\ntwo\n", true)
	require.True(t, written.Success, "%v", written.Error)
	require.True(t, written.Created)
	got := m.ReadFile("nested/example.txt")
	require.True(t, got.Success, "%v", got.Error)
	require.Equal(t, "one\ntwo\n", got.Content)
	require.True(t, m.FileExists("nested/example.txt"))
	require.Equal(t, "nested/example.txt", m.GetRelativePath(filepath.Join(root, "nested/example.txt")))
	listing := m.ListDirectory("nested")
	require.True(t, listing.Success)
	require.Equal(t, []string{"example.txt"}, listing.Files)
	patch, result := m.DiffFileContents("nested/example.txt", "one\nchanged\n")
	require.True(t, result.Success)
	require.Contains(t, patch, "-two")
	require.Contains(t, patch, "+changed")
	updated := m.WriteFile("nested/example.txt", "new", false)
	require.True(t, updated.Success)
	require.False(t, updated.Created)
}

func TestManagerReportsMissingDirectoryAndReadLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := NewManager(Config{WorkspacePath: root, MaxReadSizeByte: 3})
	require.False(t, m.ReadFile("missing").Success)
	require.False(t, m.ReadFile(".").Success)
	require.False(t, m.WriteFile("absent/child.txt", "data", false).Success)
	require.True(t, m.WriteFile("large.txt", "1234", false).Success)
	result := m.ReadFile("large.txt")
	require.False(t, result.Success)
	require.Error(t, result.Error)
	require.Error(t, NewManager(Config{WorkspacePath: filepath.Join(root, "missing")}).Initialize())
}
