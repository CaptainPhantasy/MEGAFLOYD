//go:build unix

package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPolicyReaderRejectsNonRegularFileWithoutBlocking(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(project, "MegaFloyd.md"), 0o600))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, project)
	files, err := loadContextFiles([]string{"MegaFloyd.md"}, store)
	require.ErrorContains(t, err, "not a regular file")
	require.Empty(t, files)
}

func TestPolicyReaderRejectsDirectory(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "MegaFloyd.md"), 0o700))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, project)
	files, err := loadContextFiles([]string{"MegaFloyd.md"}, store)
	require.ErrorContains(t, err, "not a regular file")
	require.Empty(t, files)
}
