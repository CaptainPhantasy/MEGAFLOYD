package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

func TestDeletionCopyActualParallelRecovery(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	for name, content := range map[string]string{
		"source": "new payload", "destination": "original payload",
		"saved-original": "original payload", "remaining": "preserved remainder",
	} {
		require.NoError(t, os.WriteFile(f.target(name), []byte(content), 0o600))
	}
	// Only disposable copy fixtures are overwritten. The deletion branch is
	// never entered, and the original destination bytes remain in saved-original.
	copyCommand := "cp source destination"
	failedCommand := "if false; then rm remaining; fi; false"
	inner := tools.NewParallelBashTool(permission.NewPermissionService(f.workingDir, true, nil), f.workingDir, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
	tool := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
	params := tools.ParallelBashParams{Commands: []tools.ParallelBashCommand{
		{Command: copyCommand, Description: "copy isolated fixture"},
		{Command: failedCommand, Description: "safe failed branch"},
	}}
	input, err := json.Marshal(params)
	require.NoError(t, err)
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "copy-parallel")
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "copy-and-failure", Name: tools.ParallelBashToolName, Input: string(input)})
	require.NoError(t, err)
	var metadata tools.ParallelBashResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(response.Metadata), &metadata))
	require.Len(t, metadata.Results, 2)
	require.Zero(t, metadata.Results[0].ExitCode)
	require.NotZero(t, metadata.Results[1].ExitCode)
	record := f.record("copy-parallel")
	require.Len(t, record.Confirmations, 2)
	require.Len(t, record.Executions, 1)
	require.Equal(t, []string{f.target("destination")}, record.Executions[0].Targets)
	require.Equal(t, "finished", record.Attempts[len(record.Attempts)-1].State)
	f.guard = deletionguard.NewGuardWithAuthority(f.dataDir, nil, f.authority)
	retry, ran := f.run("copy-parallel", failedCommand)
	require.True(t, ran, retry.Content)
	require.False(t, retry.IsError, retry.Content)
	require.Len(t, f.asked, 2)
	require.False(t, f.guard.Check(t.Context(), "copy-parallel", "repeat-copy", deletionguard.Classify(copyCommand, f.workingDir)).Allowed)
	for name, expected := range map[string]string{
		"source": "new payload", "destination": "new payload",
		"saved-original": "original payload", "remaining": "preserved remainder",
	} {
		data, err := os.ReadFile(f.target(name))
		require.NoError(t, err)
		require.Equal(t, expected, string(data), name)
	}
}

func TestDeletionCopyCompoundCannotEnterTool(t *testing.T) {
	for _, command := range []string{
		"truncate -s 0 destination; cp source destination",
		"rm destination; cp source destination",
		"cp source destination; truncate -s 0 destination",
		"cp source destination; cp other destination",
	} {
		t.Run(command, func(t *testing.T) {
			f := newDeletionFixture(t)
			f.answers = []bool{true, true}
			for _, name := range []string{"source", "other", "destination"} {
				require.NoError(t, os.WriteFile(f.target(name), []byte(name), 0o600))
			}
			// The recording body never executes shell commands. Unsupported
			// source-bearing compounds must be denied even with two answers.
			response, ran := f.run("compound-copy", command)
			require.False(t, ran, "an unbound compound copy must never enter the tool")
			require.True(t, response.IsError, response.Content)
			require.Empty(t, f.asked)
			require.Empty(t, f.record("compound-copy").Attempts)
			for _, name := range []string{"source", "other", "destination"} {
				data, err := os.ReadFile(f.target(name))
				require.NoError(t, err)
				require.Equal(t, name, string(data))
			}
		})
	}
}
