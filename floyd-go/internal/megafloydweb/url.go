package megafloydweb

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// SessionURL builds the clickable web URL for a session from a bare origin.
//
// The token rides in the fragment, so it is never sent to the server or
// recorded by a proxy. An empty session ID opens the session list.
func SessionURL(origin, sessionID, token string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSuffix(origin, "/"))
	b.WriteString("/sessions/")
	if sessionID != "" {
		b.WriteString(url.PathEscape(sessionID))
	}
	if token != "" {
		b.WriteString("#token=")
		b.WriteString(token)
	}
	return b.String()
}

// HTTPOrigin builds an http origin for a host and port.
func HTTPOrigin(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// TailnetOrigin builds the public origin for the detected Tailscale
// identity. A working MagicDNS name means HTTPS through Tailscale Serve;
// otherwise the Tailscale IP is used directly over HTTP.
func TailnetOrigin(info Tailscale, serve bool, port int) string {
	if serve && info.DNSName != "" {
		return serveURL(info.DNSName)
	}
	if info.IPv4 != "" {
		return HTTPOrigin(info.IPv4, port)
	}
	return HTTPOrigin("127.0.0.1", port)
}

// LocalOrigin builds the loopback origin.
func LocalOrigin(port int) string {
	return HTTPOrigin("127.0.0.1", port)
}
