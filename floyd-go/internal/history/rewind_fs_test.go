//go:build darwin || linux

package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise the exact verification-to-move window without timing or global test
// hooks: replace the ancestor after verification, then invoke the same bound
// file operations used by RestoreTurns.
func TestRewindForwardOperationsStayBoundAfterAncestorReplacement(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	ancestor := filepath.Join(root, "nested")
	outside := filepath.Join(base, "outside")
	moved := filepath.Join(root, "original-nested")
	require.NoError(t, os.MkdirAll(ancestor, 0o700))
	require.NoError(t, os.Mkdir(outside, 0o700))
	path := filepath.Join(ancestor, "file.txt")
	outsideFile := filepath.Join(outside, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("after\n"), 0o600))
	require.NoError(t, os.WriteFile(outsideFile, []byte("after\n"), 0o600))
	outsideInfo, err := os.Stat(outsideFile)
	require.NoError(t, err)
	workspace, err := openRewindRoot(root)
	require.NoError(t, err)
	defer workspace.Close()
	directory, err := workspace.descend("nested")
	require.NoError(t, err)
	defer directory.Close()
	recovery, err := workspace.child("recovery", true)
	require.NoError(t, err)
	defer recovery.Close()
	end, _, err := directory.snapshot("file.txt")
	require.NoError(t, err)
	file := &rewindFile{Path: path, End: end, directory: directory, name: "file.txt"}
	require.NoError(t, verifyRewindBinding(root, workspace, file))
	require.NoError(t, verifyRewindCurrent(file, false))

	require.NoError(t, os.Rename(ancestor, moved))
	require.NoError(t, os.Symlink(outside, ancestor))
	// A future check refuses the changed namespace, but the already-verified
	// move and installation cannot escape even in this narrow race window.
	require.Error(t, verifyRewindBinding(root, workspace, file))
	require.NoError(t, directory.move("file.txt", recovery, "original"))
	require.NoError(t, recovery.write("restore", []byte("before\n"), 0o600))
	require.NoError(t, recovery.link("restore", directory, "file.txt"))
	require.NoError(t, syncRewindDirectories(directory, recovery))
	content, err := os.ReadFile(outsideFile)
	require.NoError(t, err)
	require.Equal(t, "after\n", string(content))
	actualInfo, err := os.Stat(outsideFile)
	require.NoError(t, err)
	require.True(t, os.SameFile(outsideInfo, actualInfo))
	content, err = os.ReadFile(filepath.Join(moved, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "before\n", string(content))
	content, err = os.ReadFile(filepath.Join(root, "recovery", "original"))
	require.NoError(t, err)
	require.Equal(t, "after\n", string(content))
}

func TestRewindRecoveryOperationsStayBoundAfterReplacement(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	workspace, err := openRewindRoot(root)
	require.NoError(t, err)
	defer workspace.Close()
	recovery, err := workspace.child("recovery", true)
	require.NoError(t, err)
	defer recovery.Close()
	require.NoError(t, os.Rename(filepath.Join(root, "recovery"), filepath.Join(root, "original-recovery")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "recovery")))
	require.NoError(t, recovery.write("manifest.json", []byte("retained"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("original"), 0o600))
	require.NoError(t, workspace.move("file.txt", recovery, "file.original"))
	require.NoError(t, recovery.link("file.original", workspace, "file.txt"))
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
	content, err := os.ReadFile(filepath.Join(root, "original-recovery", "manifest.json"))
	require.NoError(t, err)
	require.Equal(t, "retained", string(content))
	content, err = os.ReadFile(filepath.Join(root, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
}

func TestRewindHandlesRejectSymlinksAndPreserveExistingTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "file.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside"), 0o600))
	workspace, err := openRewindRoot(root)
	require.NoError(t, err)
	defer workspace.Close()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "directory-link")))
	_, err = workspace.descend("directory-link")
	require.Error(t, err)
	require.NoError(t, os.Symlink(outsideFile, filepath.Join(root, "file-link")))
	_, _, err = workspace.snapshot("file-link")
	require.Error(t, err)
	require.Error(t, workspace.write("file-link", []byte("replacement"), 0o600))
	require.NoError(t, workspace.write("source", []byte("source"), 0o600))
	require.NoError(t, workspace.write("destination", []byte("destination"), 0o600))
	require.ErrorIs(t, workspace.move("source", workspace, "destination"), os.ErrExist)
	require.ErrorIs(t, workspace.link("source", workspace, "destination"), os.ErrExist)
	for name, expected := range map[string]string{"source": "source", "destination": "destination"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		require.Equal(t, expected, string(content))
	}
	content, err := os.ReadFile(outsideFile)
	require.NoError(t, err)
	require.Equal(t, "outside", string(content))
	_, err = openRewindRoot(filepath.Join(root, "directory-link"))
	require.Error(t, err)
}
