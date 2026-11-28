package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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

// unreachableAuthority models an approval service that is not running now but
// can be started.
type unreachableAuthority struct{}

func (unreachableAuthority) Read(context.Context, string, string) ([]byte, uint64, error) {
	return nil, 0, errors.New("connect approval authority: no such file or directory")
}

func (unreachableAuthority) CompareAndSwap(context.Context, string, string, uint64, []byte) error {
	return errors.New("connect approval authority: no such file or directory")
}

// captureDeletionGateNotes points the gate's note at a buffer for one test.
func captureDeletionGateNotes(t *testing.T) *bytes.Buffer {
	t.Helper()
	var notes bytes.Buffer
	previousWriter, previousTerminal := deletionGateNotes, deletionGateTerminal
	deletionGateNotes, deletionGateTerminal = &notes, func(io.Writer) bool { return false }
	t.Cleanup(func() { deletionGateNotes, deletionGateTerminal = previousWriter, previousTerminal })
	return &notes
}

// TestDestructiveCommandFollowsPermissionModeWhenAuthorityUnavailable covers a
// platform where the approval service cannot run. No confirmation can be read
// or recorded there, so a denial could never be lifted in any permission mode:
// the gate steps aside and leaves the call to the selected mode, with a note.
func TestDestructiveCommandFollowsPermissionModeWhenAuthorityUnavailable(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	ran := false
	inner := fantasy.NewAgentTool(tools.BashToolName, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ran = true
		return fantasy.NewTextResponse("ran"), nil
	})
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, t.TempDir())[0]
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "delete", Name: tools.BashToolName, Input: `{"command":"rm -rf /tmp/ouroboros-fixture"}`})
	require.NoError(t, err)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran, "the command must reach the tool")
	require.Equal(t, 1, strings.Count(notes.String(), "\n"), "one skipped check leaves one note line")
	require.Contains(t, notes.String(), "rm -rf /tmp/ouroboros-fixture")
	require.NotContains(t, notes.String(), "Permanent deletion is blocked")
}

// TestDestructiveCommandStaysBlockedWhenAuthorityUnreachable keeps the rule for
// a service that is merely not running: starting it lifts the condition, so
// the gate still fails closed and does not skip anything.
func TestDestructiveCommandStaysBlockedWhenAuthorityUnreachable(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unreachableAuthority{})
	notes := captureDeletionGateNotes(t)
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
	require.Contains(t, response.Content, "start the administrator-owned approval service")
	require.False(t, ran, "the destructive command must not run")
	require.Empty(t, notes.String(), "nothing was skipped, so nothing is noted")
}
