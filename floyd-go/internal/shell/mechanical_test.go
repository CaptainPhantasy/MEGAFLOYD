package shell

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/filelock"
	"github.com/stretchr/testify/require"
)

func TestMechanicalConflictingWritesKeepBackgroundLifetime(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "nested"), 0o700))
	alias := filepath.Join(dir, "alias")
	require.NoError(t, os.Symlink(filepath.Join(real, "nested"), alias))
	target := filepath.Join(real, "result")
	throughLink := alias + "/../result"
	resolved, err := filelock.Resolve(dir, throughLink)
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(expected, "result"), resolved)
	manager := GetBackgroundShellManager()
	first, err := manager.StartForSession(t.Context(), dir, nil, "printf A >> "+strconv.Quote(throughLink)+"; sleep 0.3; printf A >> "+strconv.Quote(throughLink), "first", "mechanical", "mechanical")
	require.NoError(t, err)
	t.Cleanup(func() { manager.Remove(first.ID) })
	require.Eventually(t, func() bool { b, _ := os.ReadFile(target); return string(b) == "A" }, time.Second, time.Millisecond)
	second, err := manager.StartForSession(t.Context(), dir, nil, "printf BB >> "+strconv.Quote(target), "second", "mechanical", "mechanical")
	require.NoError(t, err)
	t.Cleanup(func() { manager.Remove(second.ID) })
	// A read and an unrelated write must finish while the first write holds its lock.
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	read, err := RunAndCapture(ctx, RunOptions{Cwd: dir, Command: "cat " + strconv.Quote(target)})
	require.NoError(t, err)
	require.Equal(t, 0, read.ExitCode)
	require.Equal(t, "A", read.Output)
	other, err := RunAndCapture(ctx, RunOptions{Cwd: dir, Command: "printf independent > other"})
	require.NoError(t, err)
	require.Equal(t, 0, other.ExitCode)
	first.Wait()
	second.Wait()
	_, _, done, execErr := first.GetOutput()
	require.True(t, done)
	require.NoError(t, execErr)
	_, _, done, execErr = second.GetOutput()
	require.True(t, done)
	require.NoError(t, execErr)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "AABB", string(content))
	// Existing case aliases and hard links must share the same write lock.
	hardlink := filepath.Join(real, "hardlink")
	require.NoError(t, os.Link(target, hardlink))
	aliases := []string{hardlink}
	caseAlias := filepath.Join(real, "RESULT")
	originalInfo, err := os.Stat(target)
	require.NoError(t, err)
	if caseInfo, caseErr := os.Stat(caseAlias); caseErr == nil && os.SameFile(originalInfo, caseInfo) {
		aliases = append(aliases, caseAlias)
		t.Log("Case-insensitive filesystem alias exercised")
	}
	for _, name := range aliases {
		identity, resolveErr := filelock.Resolve(dir, name)
		require.NoError(t, resolveErr)
		release, lockErr := filelock.Acquire(t.Context(), []string{resolved})
		require.NoError(t, lockErr)
		waitCtx, cancelWait := context.WithTimeout(t.Context(), 25*time.Millisecond)
		secondRelease, secondErr := filelock.Acquire(waitCtx, []string{identity})
		cancelWait()
		if secondRelease != nil {
			secondRelease()
		}
		release()
		require.ErrorIs(t, secondErr, context.DeadlineExceeded, name)
	}
	_, err = filelock.Resolve(dir, target+"/child")
	require.Error(t, err, "filesystem read errors must not become an empty target list")
}
