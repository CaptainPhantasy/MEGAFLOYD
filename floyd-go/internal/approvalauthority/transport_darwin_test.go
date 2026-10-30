//go:build darwin

package approvalauthority

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDarwinAuthorityRefusesBeforeListenerOrStateMutation(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "private-state")
	require.NoError(t, os.Mkdir(state, 0o700))
	marker := filepath.Join(state, "existing-record")
	require.NoError(t, os.WriteFile(marker, []byte("preserve-existing-history"), 0o600))
	socket := filepath.Join(dir, "authority.sock")
	err := Serve(t.Context(), Config{SocketPath: socket, StateDir: state})
	require.ErrorIs(t, err, ErrTransportUnavailable)
	_, err = os.Lstat(socket)
	require.True(t, os.IsNotExist(err), "unsupported transport must never create a listener")
	data, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, []byte("preserve-existing-history"), data)
	entries, err := os.ReadDir(state)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no authority launch lock or new record may be created")
}

func TestDarwinAuthorityCannotAcceptOrPublishApprovalRecords(t *testing.T) {
	client := NewClient(filepath.Join(t.TempDir(), "uncontacted.sock"), 0)
	data, revision, err := client.Read(t.Context(), "namespace", "session")
	require.ErrorIs(t, err, ErrTransportUnavailable)
	require.Nil(t, data)
	require.Zero(t, revision)
	require.ErrorIs(t, client.CompareAndSwap(t.Context(), "namespace", "session", 0, []byte("forged-history")), ErrTransportUnavailable)
	result, err := exchangeAuthenticated(nil, request{Operation: "read"}, localPolicyAbsent)
	require.ErrorIs(t, err, ErrTransportUnavailable)
	require.Nil(t, result.Data)
	reader, err := authenticatedReader(nil, identity{})
	require.ErrorIs(t, err, ErrTransportUnavailable)
	require.Nil(t, reader, "Darwin must not return a raw stream as an authenticated reader")
}
