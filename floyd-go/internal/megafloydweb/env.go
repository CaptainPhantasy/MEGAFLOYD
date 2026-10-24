package megafloydweb

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/legacy-ai/floyd/internal/agent"
)

// BuildChildEnv returns base with FLOYD_CODE_ALLOWED_HOSTS extended by the
// Tailscale DNS name and IPv4 address, with FLOYD_CODE_HOME pointing at
// the canonical dataDir even when a parent home is inherited, with the authored
// identity stack in FLOYD_ROLE_ADDITIONAL.
//
// Existing allowlist entries are preserved and come first. The result has
// exactly one entry per managed key, so the child does not depend on
// duplicate-key resolution.
func BuildChildEnv(base []string, info Tailscale, dataDir string) []string {
	env := MergeAllowedHosts(base, info.DNSName, info.IPv4)
	if dataDir == "" {
		home := LookupEnv(base, homeEnv)
		if home == "" {
			home = HomeDir()
		}
		dataDir = DataDir(base, home)
	}
	env = setEnv(env, codeHomeEnv, dataDir)
	return withRolePrompt(env)
}

// withRolePrompt attaches the Go prompt layers to the child environment. The
// payload is gzip+base64 with a version tag so it survives the environment
// untouched (embedded newlines would otherwise break process spawning) and so
// the child can reject anything it does not understand.
func withRolePrompt(env []string) []string {
	prompt := strings.TrimSpace(agent.WebRolePrompt())
	if prompt == "" {
		return env
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(prompt)); err != nil {
		return env
	}
	if err := zw.Close(); err != nil {
		return env
	}
	return setEnv(env, rolePromptEnv, "gz1:"+base64.StdEncoding.EncodeToString(buf.Bytes()))
}

// setEnv removes every entry with key and appends one new entry.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		k, _, ok := strings.Cut(entry, "=")
		if ok && k == key {
			continue
		}
		out = append(out, entry)
	}
	return append(out, key+"="+value)
}

// MergeAllowedHosts appends extras to the FLOYD_CODE_ALLOWED_HOSTS value in
// base, removes duplicates, and returns one merged entry in place of the
// originals. All other entries keep their order and value.
func MergeAllowedHosts(base []string, extras ...string) []string {
	var (
		hosts []string
		out   = make([]string, 0, len(base)+1)
	)
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == allowedHostsEnv {
			hosts = append(hosts, splitHostList(value)...)
			continue
		}
		out = append(out, entry)
	}

	hosts = dedupeHosts(append(hosts, extras...))
	if len(hosts) > 0 {
		out = append(out, allowedHostsEnv+"="+strings.Join(hosts, ","))
	}
	return out
}

func splitHostList(value string) []string {
	parts := strings.Split(value, ",")
	hosts := make([]string, 0, len(parts))
	for _, part := range parts {
		if host := strings.TrimSpace(part); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func dedupeHosts(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	hosts := make([]string, 0, len(values))
	for _, value := range values {
		host := strings.TrimSpace(value)
		if host == "" {
			continue
		}
		key := strings.ToLower(host)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts
}

// AllowedHosts returns the managed allowlist entries from a built
// environment, split for command-line reuse.
func AllowedHosts(env []string) []string {
	return splitHostList(LookupEnv(env, allowedHostsEnv))
}

// errRolePromptVersion reports an environment value this build cannot read.
var errRolePromptVersion = errors.New("unsupported role prompt encoding")

// decodeRolePrompt reverses withRolePrompt's encoding. It exists so tests can
// assert the wire format the Node agent depends on.
func decodeRolePrompt(value string) (string, error) {
	raw, ok := strings.CutPrefix(value, "gz1:")
	if !ok {
		return "", errRolePromptVersion
	}
	packed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return "", err
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
