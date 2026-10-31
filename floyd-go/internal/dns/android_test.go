package dns

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolverFallsBackToReachableNameserver(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	finished := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			finished <- err
			return
		}
		defer connection.Close()
		_, err = connection.Write([]byte("resolver fixture"))
		finished <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	connection, err := dialWithFallback([]string{"127.0.0.1:0", listener.Addr().String()})(ctx, "tcp", "ignored:53")
	require.NoError(t, err)
	defer connection.Close()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(2*time.Second)))
	content, err := io.ReadAll(connection)
	require.NoError(t, err)
	require.Equal(t, "resolver fixture", string(content))
	require.NoError(t, <-finished)
}

func TestResolverReportsCancellationAndExhaustion(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	connection, err := dialWithFallback([]string{"127.0.0.1:0"})(ctx, "tcp", "ignored:53")
	require.Nil(t, connection)
	require.ErrorIs(t, err, context.Canceled)
	connection, err = dialWithFallback([]string{"127.0.0.1:0"})(t.Context(), "tcp", "ignored:53")
	require.Nil(t, connection)
	require.Error(t, err)
}
