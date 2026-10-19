package megafloydweb

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartRejectsRemoteControlOnEveryInterface(t *testing.T) {
	t.Parallel()

	_, err := Start(context.Background(), Options{
		HomeDir:       t.TempDir(),
		Port:          freePort(t),
		Remote:        true,
		RemoteControl: true,
		SkipTailscale: true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot use a non-loopback bind")
}

func TestStartRequiresTheRemoteControlRelay(t *testing.T) {
	t.Parallel()

	_, err := Start(context.Background(), Options{
		HomeDir:       t.TempDir(),
		Port:          freePort(t),
		RemoteControl: true,
		SkipTailscale: true,
		BaseEnv:       []string{"PATH=/usr/bin"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), relayEnv)
}

func TestStartFailsBeforeServeWhenTheServerIsMissing(t *testing.T) {
	t.Parallel()

	_, err := Start(context.Background(), Options{
		HomeDir:       t.TempDir(),
		CWD:           t.TempDir(),
		Executable:    filepath.Join(t.TempDir(), "megafloyd"),
		Port:          freePort(t),
		SkipTailscale: true,
		NodePath:      "/opt/node",
		ServerPath:    filepath.Join(t.TempDir(), "missing.mjs"),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "does not point at a file")
}

func TestWaitRemoteControlURLReadsTheLock(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "server", "rc.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(
		`{"pid":1,"nonce":"n","local_origin":"http://127.0.0.1:58627",`+
			`"device_id":"d","url":"https://relay.example/devices/d/","started_at":1}`,
	), 0o600))

	url, err := WaitRemoteControlURL(context.Background(), dataDir, 100*time.Millisecond)

	require.NoError(t, err)
	require.Equal(t, "https://relay.example/devices/d/", url)
}

func TestWaitRemoteControlURLTimesOut(t *testing.T) {
	t.Parallel()

	_, err := WaitRemoteControlURL(context.Background(), t.TempDir(), 50*time.Millisecond)

	require.Error(t, err)
	require.Contains(t, err.Error(), "did not publish a URL")
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}
