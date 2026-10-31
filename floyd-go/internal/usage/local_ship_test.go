package usage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalTelemetryEndpointRejectsLookalikesAndRemoteRedirect(t *testing.T) {
	for _, endpoint := range []string{"http://localhost.attacker.invalid/traces", "http://127.0.0.1.attacker.invalid/traces", "http://localhost@attacker.invalid/traces", "file:///tmp/traces", "http://192.0.2.1/traces"} {
		require.False(t, isLocalEndpoint(endpoint), endpoint)
	}
	for _, endpoint := range []string{"http://127.0.0.1:4318/v1/traces", "http://localhost:4318/v1/traces", "http://[::1]:4318/v1/traces"} {
		require.True(t, isLocalEndpoint(endpoint), endpoint)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://attacker.invalid/traces", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := Ship(server.URL, []Event{{Session: "fixture", Kind: KindPrompt}}, false)
	require.ErrorContains(t, err, "refusing to redirect")
}
