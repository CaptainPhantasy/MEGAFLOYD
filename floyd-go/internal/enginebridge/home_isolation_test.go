package enginebridge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHomeIgnoresParentHarnessStore(t *testing.T) {
	canonicalHome := t.TempDir()
	parentHome := t.TempDir()
	writeFile(t, filepath.Join(parentHome, "session_index.jsonl"), "")
	realHome := os.Getenv("HOME")
	t.Setenv("MEGAFLOYD_HOME", canonicalHome)
	t.Setenv("MEGAFLOYD_WEB_DATA", "")
	t.Setenv("FLOYD_CODE_HOME", parentHome)
	require.Equal(t, filepath.Join(canonicalHome, "web"), Home())
	require.Equal(t, realHome, os.Getenv("HOME"))
	t.Setenv("MEGAFLOYD_WEB_DATA", filepath.Join(canonicalHome, "custom-web"))
	require.Equal(t, filepath.Join(canonicalHome, "custom-web"), Home())
}
