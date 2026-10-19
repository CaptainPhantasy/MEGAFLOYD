package megafloydweb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const tailscaleStatusFixture = `{
  "Version": "1.102.4",
  "TUN": true,
  "BackendState": "Running",
  "TailscaleIPs": ["100.91.82.106", "fd7a:115c:a1e0::7628:526b"],
  "Self": {
    "ID": "node-id",
    "HostName": "Mac-mini",
    "DNSName": "mac-mini.tail58d565.ts.net.",
    "TailscaleIPs": ["100.91.82.106", "fd7a:115c:a1e0::7628:526b"],
    "Online": true
  }
}`

func TestParseStatus(t *testing.T) {
	t.Parallel()

	info, err := ParseStatus([]byte(tailscaleStatusFixture))

	require.NoError(t, err)
	require.Equal(t, "mac-mini.tail58d565.ts.net", info.DNSName)
	require.Equal(t, "100.91.82.106", info.IPv4)
	require.True(t, info.Available())
}

func TestParseStatusAcceptsDNSOnly(t *testing.T) {
	t.Parallel()

	info, err := ParseStatus([]byte(`{"Self":{"DNSName":"node.ts.net."}}`))

	require.NoError(t, err)
	require.Equal(t, "node.ts.net", info.DNSName)
	require.Empty(t, info.IPv4)
	require.True(t, info.Available())
}

func TestParseStatusRejectsGarbage(t *testing.T) {
	t.Parallel()

	_, err := ParseStatus([]byte("not json"))
	require.Error(t, err)
}

func TestParseStatusRejectsEmptySelf(t *testing.T) {
	t.Parallel()

	_, err := ParseStatus([]byte(`{"Self":{}}`))
	require.Error(t, err)
}

func TestFirstIPv4SkipsIPv6(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		"100.91.82.106",
		firstIPv4([]byte("fd7a:115c:a1e0::7628:526b\n100.91.82.106\n")),
	)
	require.Empty(t, firstIPv4([]byte("no addresses here")))
}

func TestDetectFallsBackToStatusIP(t *testing.T) {
	t.Parallel()

	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		require.Equal(t, "tailscale", name)
		switch strings.Join(args, " ") {
		case "ip -4":
			return nil, errors.New("no ipv4")
		case "status --json":
			return []byte(tailscaleStatusFixture), nil
		default:
			t.Fatalf("unexpected command: %s %s", name, strings.Join(args, " "))
			return nil, nil
		}
	}

	info, err := detect(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "mac-mini.tail58d565.ts.net", info.DNSName)
	require.Equal(t, "100.91.82.106", info.IPv4)
}

func TestDetectUsesIPCommandFirst(t *testing.T) {
	t.Parallel()

	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "ip -4":
			return []byte("100.91.82.106\n"), nil
		case "status --json":
			return []byte(`{"Self":{"DNSName":"mac-mini.tail58d565.ts.net."}}`), nil
		default:
			t.Fatalf("unexpected command: %s %s", name, strings.Join(args, " "))
			return nil, nil
		}
	}

	info, err := detect(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, "100.91.82.106", info.IPv4)
	require.Equal(t, "mac-mini.tail58d565.ts.net", info.DNSName)
}

func TestDetectUnavailable(t *testing.T) {
	t.Parallel()

	run := func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("tailscale is not running")
	}

	_, err := detect(context.Background(), run)

	require.Error(t, err)
	require.Contains(t, err.Error(), "tailscale is not available")
}
