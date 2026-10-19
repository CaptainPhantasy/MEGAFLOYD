package megafloydweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionURLUsesMagicDNS(t *testing.T) {
	t.Parallel()

	got := SessionURL("https://mac-mini.tail58d565.ts.net", "session-1", "tok123")

	require.Equal(t, "https://mac-mini.tail58d565.ts.net/sessions/session-1#token=tok123", got)
}

func TestSessionURLWithoutSession(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		"https://mac-mini.tail58d565.ts.net/sessions/",
		SessionURL("https://mac-mini.tail58d565.ts.net", "", ""),
	)
}

func TestSessionURLEscapesID(t *testing.T) {
	t.Parallel()

	got := SessionURL("http://127.0.0.1:58627", "a/b c", "")

	require.Equal(t, "http://127.0.0.1:58627/sessions/a%2Fb%20c", got)
}

func TestTailnetOriginPrefersServe(t *testing.T) {
	t.Parallel()

	info := Tailscale{IPv4: "100.91.82.106", DNSName: "mac-mini.tail58d565.ts.net."}

	require.Equal(t, "https://mac-mini.tail58d565.ts.net", TailnetOrigin(info, true, 58627))
	require.Equal(t, "http://100.91.82.106:58627", TailnetOrigin(info, false, 58627))
	require.Equal(t, "http://127.0.0.1:58627", TailnetOrigin(Tailscale{}, false, 58627))
}

func TestHTTPOriginBracketsIPv6(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http://[::1]:8080", HTTPOrigin("::1", 8080))
}

func TestLocalOrigin(t *testing.T) {
	t.Parallel()

	require.Equal(t, "http://127.0.0.1:4321", LocalOrigin(4321))
}
