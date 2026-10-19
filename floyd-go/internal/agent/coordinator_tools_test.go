package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/legacy-ai/floyd/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portedAgentToolNames is the set of agent tools ported from v5.3 in
// commit 54056f5. The donor directory also contains selfheal.go,
// progress.go, and registry.go; none of those are agent tools, so the
// ported tool count is 20.
var portedAgentToolNames = []string{
	"apply_patch",
	"complexity_estimator",
	"context_status",
	"failure_mode_enumerator",
	"get_active_diff",
	"list_symbols",
	"manage_scratchpad",
	"mcp_floyd-lab_execute_in_lab",
	"mcp_floyd-lab_spawn_lab",
	"mcp_floyd-lab_teardown_lab",
	"mcp_task_status",
	"parallel_bash",
	"project_map",
	"query_floyd_archive",
	"refactor_blueprint",
	"smart_replace",
	"symbol_index",
	"transcript_export",
	"vision",
	"workflow",
}

// newToolSetTestCoordinator builds a coordinator whose tool set has no
// environment-dependent extras: no MCP servers, no LSP, non-interactive.
// That makes buildTools deterministic across machines.
func newToolSetTestCoordinator(t *testing.T) *coordinator {
	t.Helper()

	globalDir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("FLOYD_GLOBAL_CONFIG", globalDir)
	t.Setenv("FLOYD_GLOBAL_DATA", dataDir)

	env := testEnv(t)
	floydJSON := `{
  "options": {
    "disable_default_providers": true,
    "disable_provider_auto_update": true,
    "auto_lsp": false
  },
  "providers": {"mock": {"id": "mock", "name": "Mock", "type": "openai",
    "base_url": "http://127.0.0.1:9/v1", "api_key": "test-key",
    "models": [{"id": "mock-model", "name": "Mock", "context_window": 8192, "default_max_tokens": 128}]}},
  "models": {"large": {"provider": "mock", "model": "mock-model"},
             "small": {"provider": "mock", "model": "mock-model"}}
}`
	require.NoError(t, os.WriteFile(
		filepath.Join(env.workingDir, "floyd.json"),
		[]byte(floydJSON),
		0o644,
	))

	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfg.SetupAgents()

	return &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: env.permissions,
		history:     env.history,
		filetracker: *env.filetracker,
		interactive: false,
	}
}

func buildToolNames(t *testing.T, coord *coordinator) []string {
	t.Helper()

	agentCfg := coord.cfg.Config().Agents[config.AgentCoder]
	built, err := coord.buildTools(context.Background(), agentCfg, false)
	require.NoError(t, err)

	names := make([]string, 0, len(built))
	for _, tool := range built {
		info := tool.Info()
		assert.NotEmpty(t, info.Name, "every built tool must have a name")
		assert.NotEmpty(t, info.Description, "tool %q must carry its description", info.Name)
		names = append(names, info.Name)
	}
	return names
}

// TestBuildToolsRegistersCompleteToolSet pins the full tool palette for
// the default coder agent. It exists because the 2026-10-19 audit
// counted files instead of registered constructors and concluded that
// three tools were missing when only vision is a tool.
func TestBuildToolsRegistersCompleteToolSet(t *testing.T) {
	coord := newToolSetTestCoordinator(t)

	expected := []string{
		// Core chassis tools.
		"agent",
		"agentic_fetch",
		"bash",
		"download",
		"edit",
		"fetch",
		"floyd_info",
		"floyd_logs",
		"glob",
		"grep",
		"job_kill",
		"job_output",
		"ls",
		"multiedit",
		"sourcegraph",
		"todos",
		"view",
		"write",
	}
	expected = append(expected, portedAgentToolNames...)
	slices.Sort(expected)

	actual := buildToolNames(t, coord)
	slices.Sort(actual)

	assert.Equal(t, expected, actual)
}

// TestBuildToolsHasNoDuplicateNames guards the sorted palette against a
// constructor being added twice, which would make the model's tool
// definitions ambiguous.
func TestBuildToolsHasNoDuplicateNames(t *testing.T) {
	coord := newToolSetTestCoordinator(t)

	seen := make(map[string]bool)
	for _, name := range buildToolNames(t, coord) {
		require.False(t, seen[name], "tool %q registered more than once", name)
		seen[name] = true
	}
}

// TestPortedAgentToolCount documents the corrected donor count: 21 files
// ported, of which registry.go, selfheal.go, and progress.go are not
// agent tools.
func TestPortedAgentToolCount(t *testing.T) {
	require.Len(t, portedAgentToolNames, 20)
	assert.NotContains(t, portedAgentToolNames, "registry")
	assert.NotContains(t, portedAgentToolNames, "selfheal")
	assert.NotContains(t, portedAgentToolNames, "progress")
}
