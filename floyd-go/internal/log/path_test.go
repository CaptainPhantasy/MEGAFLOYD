package log

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupRecordsConfiguredPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "floyd.log")
	Setup(file, false)
	require.Equal(t, file, Path())
}
