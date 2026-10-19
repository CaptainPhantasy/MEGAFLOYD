package megafloydweb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const serveStatusFixture = `{
  "TCP": {"443": {"HTTPS": true}},
  "Web": {
    "mac-mini.tail58d565.ts.net:443": {
      "Handlers": {
        "/": {"Proxy": "http://127.0.0.1:58627"},
        "/mcp": {"Proxy": "http://127.0.0.1:4747/mcp"}
      }
    }
  }
}`

func TestServeConfigured(t *testing.T) {
	t.Parallel()

	require.True(t, ServeConfigured([]byte(serveStatusFixture), 58627))
	require.False(t, ServeConfigured([]byte(serveStatusFixture), 4747))
	require.False(t, ServeConfigured([]byte("not json"), 58627))
	require.False(t, ServeConfigured([]byte(`{"Web":{}}`), 58627))
}

func TestEnsureServeSkipsAnExistingRoute(t *testing.T) {
	t.Parallel()

	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return []byte(serveStatusFixture), nil
	}

	require.NoError(t, ensureServe(context.Background(), run, 58627))
	require.Equal(t, []string{"tailscale serve status --json"}, calls)
}

func TestEnsureServeRegistersMissingRoute(t *testing.T) {
	t.Parallel()

	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "tailscale serve --bg 58627" {
			return nil, nil
		}
		return []byte(`{"Web":{}}`), nil
	}

	require.NoError(t, ensureServe(context.Background(), run, 58627))
	require.Equal(t, []string{
		"tailscale serve status --json",
		"tailscale serve --bg 58627",
	}, calls)
}

func TestEnsureServeReportsRegistrationFailure(t *testing.T) {
	t.Parallel()

	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "serve --bg 58627" {
			return nil, errors.New("serve is not enabled")
		}
		return []byte(`{"Web":{}}`), nil
	}

	err := ensureServe(context.Background(), run, 58627)

	require.Error(t, err)
	require.Contains(t, err.Error(), "register tailscale serve route")
}
