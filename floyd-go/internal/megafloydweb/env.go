package megafloydweb

import "strings"

// BuildChildEnv returns base with FLOYD_CODE_ALLOWED_HOSTS extended by the
// Tailscale DNS name and IPv4 address, and with FLOYD_CODE_HOME pointing at
// dataDir when the caller did not already set it.
//
// Existing allowlist entries are preserved and come first. The result has
// exactly one entry per managed key, so the child does not depend on
// duplicate-key resolution.
func BuildChildEnv(base []string, info Tailscale, dataDir string) []string {
	env := MergeAllowedHosts(base, info.DNSName, info.IPv4)
	if dataDir != "" && strings.TrimSpace(LookupEnv(env, codeHomeEnv)) == "" {
		env = setEnv(env, codeHomeEnv, dataDir)
	}
	return env
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
