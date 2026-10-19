package megafloydweb

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeHealthAcceptsTheServerEnvelope(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/healthz", r.URL.Path)
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"ok":true},"request_id":"r"}`))
	}))
	defer server.Close()

	host, port := splitURL(t, server.URL)

	require.True(t, probeHealth(context.Background(), host, port))
}

func TestProbeHealthRejectsForeignService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>not floyd</html>`))
	}))
	defer server.Close()

	host, port := splitURL(t, server.URL)

	require.False(t, probeHealth(context.Background(), host, port))
}

func TestProbeHealthRejectsClosedPort(t *testing.T) {
	t.Parallel()

	port := freePort(t)

	require.False(t, probeHealth(context.Background(), "127.0.0.1", port))
}

func splitURL(t *testing.T, raw string) (string, int) {
	t.Helper()
	host, portString, err := net.SplitHostPort(raw[len("http://"):])
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	return host, port
}
