//go:build darwin

package approvalauthority

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorityDarwinACLRejectsPartialDirectoryGrants(t *testing.T) {
	metadata := "drwxr-xr-x+ 2 root wheel 64 Oct 30 00:00 /protected\n"
	require.NoError(t, inspectDarwinACLListing(metadata, 1))
	require.NoError(t, inspectDarwinACLListing(metadata+" 0: group:everyone deny delete\n", 1))
	for _, grant := range []string{"write", "append", "delete_child", "read", "execute"} {
		require.ErrorContains(t, inspectDarwinACLListing(metadata+" 0: user:nobody allow "+grant+"\n", 1), "allow ACL")
	}
	require.ErrorContains(t, inspectDarwinACLListing(metadata+" 0: group:staff inherited allow write\n", 1), "allow ACL")
	require.Error(t, inspectDarwinACLListing(metadata+" 0: unsupported ACL evidence\n", 1))
	require.Error(t, inspectDarwinACLListing("", 1))
	require.Error(t, inspectDarwinACLListing(metadata, 2))
}
