package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBashToolSSHUsesNormalPermission(t *testing.T) {
	tool, perms := newBashToolWithRecordingPerms(t.TempDir(), true)
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "ssh-verification")
	resp := runBashTool(t, tool, ctx, BashParams{
		Description: "check SSH client",
		Command:     "ssh -V",
	})
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "OpenSSH")
	require.Equal(t, 1, perms.requestCount)

	perms.allow = false
	resp = runBashTool(t, tool, ctx, BashParams{
		Description: "denied SSH client",
		Command:     "ssh -V",
	})
	require.Contains(t, resp.Content, "User denied permission")
	require.Equal(t, 2, perms.requestCount)
}
