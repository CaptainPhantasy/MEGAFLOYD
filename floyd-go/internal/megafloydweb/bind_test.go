package megafloydweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChooseBindDefaultsToLoopback(t *testing.T) {
	t.Parallel()

	got := ChooseBind(BindOptions{})

	require.Equal(t, Bind{Host: "127.0.0.1", Mode: BindLoopback}, got)
}

func TestChooseBindUsesTailscaleIP(t *testing.T) {
	t.Parallel()

	got := ChooseBind(BindOptions{Tailscale: Tailscale{IPv4: "100.91.82.106"}})

	require.Equal(t, Bind{Host: "100.91.82.106", Mode: BindTailscale}, got)
}

func TestChooseBindServeKeepsLoopback(t *testing.T) {
	t.Parallel()

	got := ChooseBind(BindOptions{
		Serve:     true,
		Tailscale: Tailscale{IPv4: "100.91.82.106", DNSName: "mac-mini.tail58d565.ts.net"},
	})

	require.Equal(t, Bind{Host: "127.0.0.1", Mode: BindLoopback}, got)
}

func TestChooseBindRemoteIsExplicit(t *testing.T) {
	t.Parallel()

	got := ChooseBind(BindOptions{
		Remote:    true,
		Tailscale: Tailscale{IPv4: "100.91.82.106", DNSName: "mac-mini.tail58d565.ts.net"},
	})

	require.Equal(t, Bind{Host: "0.0.0.0", Mode: BindAll}, got)
}

func TestChooseBindRemoteControlForcesLoopback(t *testing.T) {
	t.Parallel()

	got := ChooseBind(BindOptions{
		Remote:        true,
		RemoteControl: true,
		Tailscale:     Tailscale{IPv4: "100.91.82.106"},
	})

	require.Equal(t, Bind{Host: "127.0.0.1", Mode: BindLoopback}, got)
}

func TestIsLoopback(t *testing.T) {
	t.Parallel()

	require.True(t, IsLoopback("127.0.0.1"))
	require.True(t, IsLoopback("localhost"))
	require.False(t, IsLoopback("100.91.82.106"))
	require.False(t, IsLoopback("0.0.0.0"))
}
