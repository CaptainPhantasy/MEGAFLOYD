package megafloydweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArgsLoopback(t *testing.T) {
	t.Parallel()

	spec := commandSpec{
		Node:   "/opt/node",
		Server: "/opt/floyd-code/dist/main.mjs",
		Port:   58627,
		Bind:   Bind{Host: "127.0.0.1", Mode: BindLoopback},
	}

	require.Equal(t, []string{
		"/opt/floyd-code/dist/main.mjs",
		"web",
		"--no-open",
		"--port", "58627",
		"--host", "127.0.0.1",
	}, spec.args())
}

func TestArgsTailscaleAddsInsecureTLS(t *testing.T) {
	t.Parallel()

	spec := commandSpec{
		Server: "/srv/main.mjs",
		Port:   58627,
		Bind:   Bind{Host: "100.91.82.106", Mode: BindTailscale},
	}

	require.Contains(t, spec.args(), "--insecure-no-tls")
}

func TestArgsRemoteControl(t *testing.T) {
	t.Parallel()

	spec := commandSpec{
		Server:        "/srv/main.mjs",
		Port:          58627,
		Bind:          Bind{Host: "127.0.0.1", Mode: BindLoopback},
		RemoteControl: true,
	}

	require.Contains(t, spec.args(), "--rc")
}

func TestArgsRepeatsAllowedHosts(t *testing.T) {
	t.Parallel()

	spec := commandSpec{
		Server:       "/srv/main.mjs",
		Port:         58627,
		Bind:         Bind{Host: "127.0.0.1", Mode: BindLoopback},
		AllowedHosts: []string{"mac-mini.tail58d565.ts.net", "100.91.82.106", "", "-bad"},
	}

	args := spec.args()

	require.Contains(t, args, "--allowed-host")
	require.Contains(t, args, "mac-mini.tail58d565.ts.net")
	require.Contains(t, args, "100.91.82.106")
	require.NotContains(t, args, "-bad")
	require.Equal(t, 2, countValue(args, "--allowed-host"))
}

func TestNewCommandBuildsWithoutStarting(t *testing.T) {
	t.Parallel()

	cmd, err := newCommand(commandSpec{
		Node:   "/opt/node",
		Server: "/srv/main.mjs",
		Dir:    "/workspace",
		Env:    []string{"A=B"},
		Port:   58627,
		Bind:   Bind{Host: "127.0.0.1", Mode: BindLoopback},
	})

	require.NoError(t, err)
	require.Nil(t, cmd.Process, "building must not start the server")
	require.Equal(t, "/opt/node", cmd.Path)
	require.Equal(t, "/workspace", cmd.Dir)
	require.Equal(t, []string{"A=B", "ELECTRON_RUN_AS_NODE=1"}, cmd.Env)
}

func TestNewCommandForcesElectronRunAsNode(t *testing.T) {
	t.Parallel()

	cmd, err := newCommand(commandSpec{
		Node:   "/opt/node",
		Server: "/srv/main.mjs",
		Port:   58627,
		Bind:   Bind{Host: "127.0.0.1", Mode: BindLoopback},
		Env:    []string{"ELECTRON_RUN_AS_NODE=0"},
	})

	require.NoError(t, err)
	require.Contains(t, cmd.Env, "ELECTRON_RUN_AS_NODE=1")
	require.NotContains(t, cmd.Env, "ELECTRON_RUN_AS_NODE=0")
}

func TestNewCommandValidation(t *testing.T) {
	t.Parallel()

	valid := commandSpec{
		Node:   "/opt/node",
		Server: "/srv/main.mjs",
		Port:   58627,
		Bind:   Bind{Host: "127.0.0.1", Mode: BindLoopback},
	}

	tests := map[string]func(*commandSpec){
		"missing node":   func(s *commandSpec) { s.Node = "" },
		"missing server": func(s *commandSpec) { s.Server = "" },
		"bad port":       func(s *commandSpec) { s.Port = 0 },
		"missing bind":   func(s *commandSpec) { s.Bind.Host = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := valid
			mutate(&spec)
			_, err := newCommand(spec)
			require.Error(t, err)
		})
	}
}

func countValue(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
