package megafloydweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const detectTimeout = 3 * time.Second

// Tailscale is the subset of the local Tailscale identity the web handoff
// needs.
type Tailscale struct {
	IPv4    string
	DNSName string
}

// Available reports whether at least one Tailscale address was found.
func (t Tailscale) Available() bool {
	return t.IPv4 != "" || t.DNSName != ""
}

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Detect asks the local Tailscale client for the IPv4 address and MagicDNS
// name. The whole probe is capped so starting web mode never hangs on a
// wedged daemon.
func Detect(ctx context.Context) (Tailscale, error) {
	return detect(ctx, runCommand)
}

func detect(ctx context.Context, run commandRunner) (Tailscale, error) {
	ctx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()

	var info Tailscale
	ipOut, ipErr := run(ctx, "tailscale", "ip", "-4")
	if ipErr == nil {
		info.IPv4 = firstIPv4(ipOut)
	}

	statusOut, statusErr := run(ctx, "tailscale", "status", "--json")
	if statusErr == nil {
		parsed, err := ParseStatus(statusOut)
		if err == nil {
			if info.IPv4 == "" {
				info.IPv4 = parsed.IPv4
			}
			info.DNSName = parsed.DNSName
		} else {
			statusErr = err
		}
	}

	if !info.Available() {
		joined := errors.Join(ipErr, statusErr)
		if joined == nil {
			joined = errors.New("tailscale returned no address")
		}
		return Tailscale{}, fmt.Errorf("tailscale is not available: %w", joined)
	}
	return info, nil
}

// ParseStatus extracts Self.DNSName and the first IPv4 address from the
// `tailscale status --json` document.
func ParseStatus(data []byte) (Tailscale, error) {
	var status struct {
		Self struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return Tailscale{}, fmt.Errorf("parse tailscale status: %w", err)
	}

	dnsName := strings.TrimSuffix(strings.TrimSpace(status.Self.DNSName), ".")
	ipv4 := ""
	for _, candidate := range status.Self.TailscaleIPs {
		if parsed := net.ParseIP(strings.TrimSpace(candidate)); parsed != nil && parsed.To4() != nil {
			ipv4 = parsed.String()
			break
		}
	}

	if dnsName == "" && ipv4 == "" {
		return Tailscale{}, errors.New("tailscale status has no Self.DNSName or IPv4 address")
	}
	return Tailscale{IPv4: ipv4, DNSName: dnsName}, nil
}

func firstIPv4(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if parsed := net.ParseIP(line); parsed != nil && parsed.To4() != nil {
			return parsed.String()
		}
	}
	return ""
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
