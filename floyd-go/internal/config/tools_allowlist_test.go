package config

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portedAgentToolNames mirrors the v5.3 tool set registered by
// coordinator.buildTools. registry.go, selfheal.go, and progress.go are
// not agent tools, so the ported count is 20.
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

// TestAllToolNamesHasNoDuplicates keeps the coder allowlist unambiguous.
func TestAllToolNamesHasNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, name := range allToolNames() {
		require.False(t, seen[name], "tool %q listed more than once", name)
		seen[name] = true
	}
}

// TestAllToolNamesCoversPortedTools ensures every ported agent tool is
// allowed by default; registering a constructor without allowlisting it
// silently hides the tool from the coder agent.
func TestAllToolNamesCoversPortedTools(t *testing.T) {
	names := allToolNames()
	require.Len(t, portedAgentToolNames, 20)
	for _, want := range portedAgentToolNames {
		assert.True(t, slices.Contains(names, want), "allToolNames missing %q", want)
	}
}

// TestAllToolNamesCoversPlanTools keeps both plan-mode tools in the
// default coder allowlist.
func TestAllToolNamesCoversPlanTools(t *testing.T) {
	names := allToolNames()
	for _, want := range []string{"enter_plan_mode", "exit_plan_mode"} {
		assert.True(t, slices.Contains(names, want), "allToolNames missing %q", want)
	}
}

// TestSetupAgentsAllowsPortedTools checks the default coder agent and
// keeps the task agent on its read-only subset.
func TestSetupAgentsAllowsPortedTools(t *testing.T) {
	cfg := &Config{Options: &Options{DisabledTools: []string{}}}
	cfg.SetupAgents()

	coder, ok := cfg.Agents[AgentCoder]
	require.True(t, ok)
	for _, want := range portedAgentToolNames {
		assert.Contains(t, coder.AllowedTools, want, "coder allowlist missing %q", want)
	}

	task, ok := cfg.Agents[AgentTask]
	require.True(t, ok)
	// resolveReadOnlyTools preserves allToolNames order.
	readOnly := []string{
		"lsp_symbols",
		"lsp_definition",
		"lsp_call_hierarchy",
		"glob",
		"grep",
		"ls",
		"sourcegraph",
		"view",
	}
	assert.Equal(t, readOnly, task.AllowedTools)
}
