package environment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMCPDiscoveryRequiresServerEntryPoint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	valid := filepath.Join(root, "fixture-server")
	require.NoError(t, os.MkdirAll(filepath.Join(valid, "src"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(valid, "src", "index.ts"), []byte("export {};"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "empty-server"), 0o700))
	state := &EnvironmentState{MCPServers: map[string]string{}}
	discoverMCPServers(state, root)
	require.Equal(t, map[string]string{"fixture": valid}, state.MCPServers)
	require.True(t, PathExists(valid))
	require.False(t, PathExists(filepath.Join(root, "missing")))
	require.Equal(t, valid, discoverComponent(root, "missing", "fixture-server"))
	require.Empty(t, discoverComponent(root, "missing"))
}

func TestEnvironmentSummaryDistinguishesInstalledAndConfigured(t *testing.T) {
	t.Parallel()
	require.Equal(t, "Environment: unknown", FormatSummary(nil))
	state := &EnvironmentState{TotalComponents: 2, Components: map[string]Component{
		"cli":     {Name: "MegaFloyd", Type: "cli", Path: "/fixture/cli", State: ComponentInstalled},
		"desktop": {Name: "Desktop", Type: "desktop", Path: "/fixture/app", State: ComponentConfigured},
	}}
	require.Equal(t, "Environment: 2 components\n  ✓ MegaFloyd: /fixture/cli\n  ○ Desktop: /fixture/app\n", FormatSummary(state))
}
