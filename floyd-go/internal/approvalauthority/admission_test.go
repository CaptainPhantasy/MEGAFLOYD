package approvalauthority

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthorityBoundsConnectionsBeforeAuthentication(t *testing.T) {
	requireSupportedAuthorityTransport(t)
	socket := filepath.Join(t.TempDir(), "authority.sock")
	ctx, cancel := context.WithCancel(t.Context())
	var active, peak atomic.Int32
	entered := make(chan struct{}, maxConcurrentConnections+1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	s := &server{
		config: Config{SocketPath: socket},
		authorize: func(*net.UnixConn) error {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			entered <- struct{}{}
			<-release
			return os.ErrPermission
		},
	}
	done := make(chan error, 1)
	go func() { done <- s.serve(ctx) }()
	var clients []*net.UnixConn
	defer func() {
		releaseOnce.Do(func() { close(release) })
		for _, conn := range clients {
			_ = conn.Close()
		}
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("authority workers failed to stop")
		}
	}()
	require.Eventually(t, func() bool {
		_, err := os.Lstat(socket)
		return err == nil
	}, 3*time.Second, time.Millisecond)
	dial := func() *net.UnixConn {
		conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
		require.NoError(t, err)
		clients = append(clients, conn)
		return conn
	}
	for range maxConcurrentConnections {
		dial()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("admitted connection never reached fake authorizer")
		}
	}
	// A bounded burst beyond capacity must close immediately, without invoking
	// even the fake authorizer or waiting for an existing connection to finish.
	for range maxConcurrentConnections {
		conn := dial()
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		var one [1]byte
		n, err := conn.Read(one[:])
		require.Zero(t, n)
		require.Error(t, err)
		if netErr, ok := err.(net.Error); ok {
			require.False(t, netErr.Timeout(), "excess connection must be rejected immediately")
		}
	}
	require.Equal(t, int32(maxConcurrentConnections), active.Load())
	require.Equal(t, int32(maxConcurrentConnections), peak.Load())
	require.Empty(t, entered, "overflow must not start authentication")
	releaseOnce.Do(func() { close(release) })
	for _, conn := range clients[:maxConcurrentConnections] {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
		var result response
		require.NoError(t, json.NewDecoder(conn).Decode(&result))
		require.Contains(t, result.Error, "rejected peer")
	}
	// Error exits release capacity, allowing a later legitimate attempt.
	require.Eventually(t, func() bool {
		conn := dial()
		_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var result response
		err := json.NewDecoder(conn).Decode(&result)
		_ = conn.Close()
		return err == nil && result.Error != ""
	}, 3*time.Second, time.Millisecond)
	require.LessOrEqual(t, peak.Load(), int32(maxConcurrentConnections))
}
