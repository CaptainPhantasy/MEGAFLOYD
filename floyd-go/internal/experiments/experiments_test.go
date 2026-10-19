package experiments

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()

	noEnv := func(string) string { return "" }

	source, enabled := resolve(nil, GhostText, noEnv)
	require.Equal(t, SourceDefault, source)
	require.True(t, enabled)

	source, enabled = resolve(map[string]bool{GhostText: false}, GhostText, noEnv)
	require.Equal(t, SourceConfig, source)
	require.False(t, enabled)

	env := map[string]string{EnvName(GhostText): "1"}
	lookup := func(key string) string { return env[key] }
	source, enabled = resolve(map[string]bool{GhostText: false}, GhostText, lookup)
	require.Equal(t, SourceEnv, source)
	require.True(t, enabled)

	env[EnvName(GhostText)] = "0"
	source, enabled = resolve(map[string]bool{GhostText: true}, GhostText, lookup)
	require.Equal(t, SourceEnv, source)
	require.False(t, enabled)

	delete(env, EnvName(GhostText))
	env[MasterEnv] = "true"
	source, enabled = resolve(nil, GhostText, lookup)
	require.Equal(t, SourceMaster, source)
	require.True(t, enabled)
}

func TestSetCopiesAndValidates(t *testing.T) {
	t.Parallel()

	original := map[string]bool{GhostText: true}
	updated, ok := Set(original, GhostText, false)
	require.True(t, ok)
	require.False(t, updated[GhostText])
	require.True(t, original[GhostText], "Set must not mutate the input map")

	_, ok = Set(original, "missing", true)
	require.False(t, ok)
}

func TestListReportsKnownFlags(t *testing.T) {
	t.Setenv(EnvName(GhostText), "")
	t.Setenv(MasterEnv, "")

	states := List(map[string]bool{GhostText: false})
	require.Len(t, states, 1)
	require.Equal(t, GhostText, states[0].ID)
	require.False(t, states[0].Enabled)
	require.Equal(t, SourceConfig, states[0].Source)
}
