//go:build darwin || linux

package approvalauthority

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorityRefusesPrivilegedEngineAccounts(t *testing.T) {
	require.ErrorContains(t, validateEngineAccount(0), "root engine")
	for _, group := range []string{"admin", "sudo", "wheel", "docker", "lxd", "incus", "disk"} {
		require.True(t, privilegedGroup(group))
	}
	require.False(t, privilegedGroup("staff"))
}

func TestAuthoritySudoPolicyDenialRequiresExactAccountAndWholeOutput(t *testing.T) {
	require.True(t, sudoDeniedOutput("nobody", []byte("User nobody is not allowed to run sudo on test-host.\n")))
	require.True(t, sudoDeniedOutput("nobody", []byte("User nobody is not allowed to run sudo on test-host.")))
	for _, output := range []string{
		"", "User other is not allowed to run sudo on test-host.\n",
		"User nobody may run the following commands on test-host:\n    (ALL) NOPASSWD: ALL\n",
		"User nobody is not allowed to run sudo on test-host.\n    (root) NOPASSWD: ALL\n",
		"warning\nUser nobody is not allowed to run sudo on test-host.\n",
	} {
		require.False(t, sudoDeniedOutput("nobody", []byte(output)), output)
	}
}

func TestAuthorityLocalUnknownDefersWithoutClaimingAbsence(t *testing.T) {
	for _, output := range []string{"sudo: a password is required\n", "", "policy query unavailable"} {
		status, err := classifyLocalSudoPolicy("fixture", []byte(output), os.ErrNotExist)
		require.NoError(t, err)
		require.Equal(t, localPolicyDeferred, status)
	}
	status, err := classifyLocalSudoPolicy("fixture", []byte("User fixture is not allowed to run sudo on test-host.\n"), nil)
	require.NoError(t, err)
	require.Equal(t, localPolicyAbsent, status)
	_, err = classifyLocalSudoPolicy("fixture", []byte("User fixture may run the following commands on test-host:\n    (root) NOPASSWD: /bin/sh\n"), nil)
	require.ErrorContains(t, err, "visible sudo grants")
}
