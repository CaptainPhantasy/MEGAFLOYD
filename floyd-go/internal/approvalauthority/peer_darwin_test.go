//go:build darwin

package approvalauthority

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorityRejectsUnhardenedExecutableRuntime(t *testing.T) {
	// Ordinary go-test binaries have no hardened signature. A matching path and
	// content hash alone must not authorize an injectable process.
	require.ErrorContains(t, verifyPeerRuntime(os.Getpid(), os.Args[0]), "hardened")
}

func TestAuthorityKernelPeerExecutableMatchesRunningProcess(t *testing.T) {
	actual, err := peerExecutable(os.Getpid())
	require.NoError(t, err)
	expected, err := os.Executable()
	require.NoError(t, err)
	expected, err = filepath.EvalSymlinks(expected)
	require.NoError(t, err)
	actual, err = filepath.EvalSymlinks(actual)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
