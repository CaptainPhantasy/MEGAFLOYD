package megafloydweb

// BindMode describes which interface the Node server listens on.
type BindMode int

const (
	// BindLoopback listens on 127.0.0.1 only. This is the default and the
	// required mode when Tailscale Serve fronts the server.
	BindLoopback BindMode = iota
	// BindTailscale listens on the node's Tailscale IPv4 address only.
	BindTailscale
	// BindAll listens on every interface. It is opt-in only.
	BindAll
)

func (m BindMode) String() string {
	switch m {
	case BindLoopback:
		return "loopback"
	case BindTailscale:
		return "tailscale"
	case BindAll:
		return "all"
	default:
		return "unknown"
	}
}

// Bind is the chosen listen host and its mode.
type Bind struct {
	Host string
	Mode BindMode
}

// BindOptions selects a listen address.
type BindOptions struct {
	// Remote is the explicit all-interfaces request.
	Remote bool
	// RemoteControl forces loopback: the Node server refuses --rc on a
	// non-loopback bind.
	RemoteControl bool
	// Serve reports that Tailscale Serve already fronts the loopback port.
	Serve bool
	// Tailscale is the detected identity.
	Tailscale Tailscale
}

// ChooseBind picks the listen host. The order is deliberate:
//
//   - remote control, loopback, and a working Serve route bind loopback;
//   - otherwise the Tailscale IPv4 is used so the tailnet reaches the
//     server without exposing it to the LAN;
//   - 0.0.0.0 is returned only for an explicit remote request.
func ChooseBind(opts BindOptions) Bind {
	switch {
	case opts.RemoteControl:
		return Bind{Host: "127.0.0.1", Mode: BindLoopback}
	case opts.Remote:
		return Bind{Host: "0.0.0.0", Mode: BindAll}
	case opts.Serve && opts.Tailscale.DNSName != "":
		return Bind{Host: "127.0.0.1", Mode: BindLoopback}
	case opts.Tailscale.IPv4 != "":
		return Bind{Host: opts.Tailscale.IPv4, Mode: BindTailscale}
	default:
		return Bind{Host: "127.0.0.1", Mode: BindLoopback}
	}
}

// IsLoopback reports whether host is a loopback address.
func IsLoopback(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
