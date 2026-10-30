package prompt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPolicyReaderRejectsAncestorSymlink(t *testing.T) {
	project := t.TempDir()
	child := filepath.Join(project, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(project, "package.json"), []byte(`{}`), 0o600))
	secret := filepath.Join(t.TempDir(), "private-fixture")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE_SECRET_FIXTURE"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(project, "MegaFloyd.md")))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, child)
	files, err := loadContextFiles(projectPolicyPaths(child), store)
	require.ErrorContains(t, err, "reading policy")
	require.Empty(t, files, "a symlink must never put its target bytes in model context")
}

func TestPolicyReaderRejectsOversizedFile(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "MegaFloyd.md"), bytes.Repeat([]byte("x"), (1<<20)+1), 0o600))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, project)
	files, err := loadContextFiles([]string{"MegaFloyd.md"}, store)
	require.ErrorContains(t, err, "reading policy")
	require.Empty(t, files)
}

func TestPolicyReaderPreservesRegularFileBytes(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "mEgAfLoYd.Md")
	content := "Regular policy\r\nwith exact spacing.  \n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	store := config.NewTestStoreWithWorkingDir(&config.Config{Options: &config.Options{}}, project)
	files, err := loadContextFiles([]string{"MegaFloyd.md"}, store)
	require.NoError(t, err)
	require.Equal(t, []ContextFile{{Path: path, Content: content}}, files)
}
