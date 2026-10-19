package megafloydweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const serveTimeout = 5 * time.Second

// ServeConfigured reports whether `tailscale serve status` already routes
// the tailnet root to the given loopback port. The status document is the
// JSON produced by `tailscale serve status --json`.
func ServeConfigured(data []byte, port int) bool {
	var status struct {
		Web map[string]struct {
			Handlers map[string]struct {
				Proxy string `json:"Proxy"`
			} `json:"Handlers"`
		} `json:"Web"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return false
	}
	for _, site := range status.Web {
		handler, ok := site.Handlers["/"]
		if !ok {
			continue
		}
		target, err := url.Parse(handler.Proxy)
		if err != nil || target.Port() == "" {
			continue
		}
		targetPort, err := strconv.Atoi(target.Port())
		if err != nil || targetPort != port {
			continue
		}
		switch target.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return true
		}
	}
	return false
}

// EnsureServe makes sure the tailnet root publishes the loopback web
// server. It inspects the existing configuration first, so an already
// working route is not rewritten on every start.
func EnsureServe(ctx context.Context, port int) error {
	return ensureServe(ctx, runCommand, port)
}

func ensureServe(ctx context.Context, run commandRunner, port int) error {
	ctx, cancel := context.WithTimeout(ctx, serveTimeout)
	defer cancel()

	if out, err := run(ctx, "tailscale", "serve", "status", "--json"); err == nil {
		if ServeConfigured(out, port) {
			return nil
		}
	}

	if _, err := run(ctx, "tailscale", "serve", "--bg", strconv.Itoa(port)); err != nil {
		return fmt.Errorf("register tailscale serve route: %w", err)
	}
	return nil
}

// serveURL is the public tailnet origin for a MagicDNS name. Tailscale
// Serve always terminates TLS on the default HTTPS port.
func serveURL(dnsName string) string {
	return "https://" + strings.TrimSuffix(dnsName, ".")
}
