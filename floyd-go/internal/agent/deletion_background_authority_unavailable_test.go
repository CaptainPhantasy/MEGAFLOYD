package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/approvalauthority"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/stretchr/testify/require"
)

// unavailableAuthority models a platform where the protected approval
// authority transport is unsupported.
type unavailableAuthority struct{}

func (unavailableAuthority) Read(context.Context, string, string) ([]byte, uint64, error) {
	return nil, 0, approvalauthority.ErrTransportUnavailable
}

func (unavailableAuthority) CompareAndSwap(context.Context, string, string, uint64, []byte) error {
	return approvalauthority.ErrTransportUnavailable
}

func TestJobOutputSurvivesUnavailableApprovalAuthority(t *testing.T) {
	for _, toolName := range []string{tools.JobOutputToolName, tools.JobKillToolName} {
		t.Run(toolName, func(t *testing.T) {
			guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
			inner := fantasy.NewAgentTool(toolName, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("job result retained"), nil
			})
			wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, t.TempDir())[0]
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
			response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "read", Name: toolName, Input: `{"shell_id":"001"}`})
			require.NoError(t, err, "a reconciliation failure must not fail the agent run")
			require.False(t, response.IsError)
			require.Contains(t, response.Content, "job result retained")
		})
	}
}

func TestDestructiveCommandStaysBlockedWhenAuthorityUnavailable(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	ran := false
	inner := fantasy.NewAgentTool(tools.BashToolName, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ran = true
		return fantasy.NewTextResponse("ran"), nil
	})
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, t.TempDir())[0]
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "delete", Name: tools.BashToolName, Input: `{"command":"rm -rf /tmp/ouroboros-fixture"}`})
	require.NoError(t, err, "a blocked deletion is a tool result, not a run failure")
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "Permanent deletion is blocked")
	require.False(t, ran, "the destructive command must not run")
}
