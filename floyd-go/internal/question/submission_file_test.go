package question

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishSubmissionReplacesOnlyAfterSuccessfulWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	destination := filepath.Join(dir, "answer.json")
	require.NoError(t, os.WriteFile(destination, []byte("previous answer"), 0o600))
	source := filepath.Join(dir, "pending")
	file, err := os.OpenFile(source, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString("new answer")
	require.NoError(t, err)
	require.NoError(t, file.Sync())
	require.NoError(t, file.Close())
	require.NoError(t, publishSubmission(source, destination))
	actual, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "new answer", string(actual))
	_, err = os.Stat(source)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.ErrorIs(t, publishSubmission(source, destination), os.ErrNotExist)
	actual, err = os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "new answer", string(actual), "failed publication preserves the previous answer")
}

func TestPublishSubmissionRetainsStagedAnswerWhenDestinationIsInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "pending")
	require.NoError(t, os.WriteFile(source, []byte("preserve this answer"), 0o600))
	for _, destination := range []string{filepath.Join(dir, "missing-parent", "answer.json"), dir} {
		require.Error(t, publishSubmission(source, destination))
		actual, err := os.ReadFile(source)
		require.NoError(t, err)
		require.Equal(t, "preserve this answer", string(actual))
	}
}
