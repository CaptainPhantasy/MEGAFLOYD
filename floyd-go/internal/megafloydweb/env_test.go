package megafloydweb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildChildEnvAppendsTailscaleHosts(t *testing.T) {
	t.Parallel()

	base := []string{
		"PATH=/usr/bin",
		"FLOYD_CODE_ALLOWED_HOSTS=localhost, app.example.test",
		"HOME=/Users/owner",
	}
	info := Tailscale{IPv4: "100.91.82.106", DNSName: "mac-mini.tail58d565.ts.net"}

	env := BuildChildEnv(base, info, "/Users/owner/.megafloyd/web")

	require.Contains(t, env, "PATH=/usr/bin")
	require.Contains(t, env, "HOME=/Users/owner")
	require.Equal(
		t,
		"localhost,app.example.test,mac-mini.tail58d565.ts.net,100.91.82.106",
		LookupEnv(env, allowedHostsEnv),
	)
	require.Equal(t, "/Users/owner/.megafloyd/web", LookupEnv(env, codeHomeEnv))
	require.Equal(t, 1, entryCount(env, allowedHostsEnv), "one allowlist entry")
	require.Equal(t, 1, entryCount(env, codeHomeEnv), "one code home entry")
}

func TestBuildChildEnvDedupesHosts(t *testing.T) {
	t.Parallel()

	base := []string{"FLOYD_CODE_ALLOWED_HOSTS=MAC-MINI.tail58d565.ts.net"}
	info := Tailscale{IPv4: "100.91.82.106", DNSName: "mac-mini.tail58d565.ts.net"}

	env := BuildChildEnv(base, info, "")

	require.Equal(
		t,
		"MAC-MINI.tail58d565.ts.net,100.91.82.106",
		LookupEnv(env, allowedHostsEnv),
	)
	require.Empty(t, LookupEnv(env, codeHomeEnv))
}

func TestBuildChildEnvReplacesEmptyCodeHome(t *testing.T) {
	t.Parallel()

	base := []string{"FLOYD_CODE_HOME=", "PATH=/usr/bin"}
	env := BuildChildEnv(base, Tailscale{}, "/megafloyd/web")

	require.Equal(t, "/megafloyd/web", LookupEnv(env, codeHomeEnv))
	require.Equal(t, 1, entryCount(env, codeHomeEnv), "one code home entry")
}

func TestBuildChildEnvKeepsExistingCodeHome(t *testing.T) {
	t.Parallel()

	base := []string{"FLOYD_CODE_HOME=/tmp/floyd-code-home"}
	env := BuildChildEnv(base, Tailscale{}, "/ignored")

	require.Equal(t, "/tmp/floyd-code-home", LookupEnv(env, codeHomeEnv))
	require.Equal(t, "/tmp/floyd-code-home", DataDir(base, "/home"))
}

func TestAllowedHostsSplitsTheMergedValue(t *testing.T) {
	t.Parallel()

	env := MergeAllowedHosts(nil, "a.ts.net", "100.1.2.3")

	require.Equal(t, []string{"a.ts.net", "100.1.2.3"}, AllowedHosts(env))
}

func entryCount(env []string, key string) int {
	count := 0
	for _, entry := range env {
		k, _, ok := strings.Cut(entry, "=")
		if ok && k == key {
			count++
		}
	}
	return count
}
