package permission

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePathAllowsWorkingDir(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	svc := NewPermissionService(work, true, nil)

	require.NoError(t, svc.ValidatePath(filepath.Join(work, "main.go")))
	require.NoError(t, svc.ValidatePath(work))
}

func TestValidatePathAllowsAdditionalDir(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	extra := t.TempDir()
	svc := NewPermissionService(work, true, nil)
	svc.SetAdditionalDirs([]string{extra})

	require.NoError(t, svc.ValidatePath(filepath.Join(extra, "shared.go")))
	require.NoError(t, svc.ValidatePath(filepath.Join(extra, "new", "nested", "file.go")))
}

func TestValidatePathDeniesOutside(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	outside := t.TempDir()
	svc := NewPermissionService(work, true, nil)

	err := svc.ValidatePath(filepath.Join(outside, "secret.txt"))
	require.Error(t, err)
	var pathErr *PathOutsideError
	require.ErrorAs(t, err, &pathErr)
	require.Contains(t, err.Error(), "outside the allowed workspace directories")
	require.Contains(t, err.Error(), "/add-dir")
}

func TestValidatePathRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o600))
	link := filepath.Join(work, "link")
	require.NoError(t, os.Symlink(outside, link))

	svc := NewPermissionService(work, true, nil)
	require.Error(t, svc.ValidatePath(filepath.Join(link, "secret.txt")))
}

func TestSetAdditionalDirsNormalizesAndDeduplicates(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	svc := NewPermissionService(work, true, nil)
	svc.SetAdditionalDirs([]string{"", "  ", "relative", work, work + "/."})

	dirs := svc.AdditionalDirs()
	require.Equal(t, []string{filepath.Join(work, "relative"), work}, dirs)
}
