package agent

import (
	"bytes"
	"context"
	"errors"
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

// unreachableAuthority models an approval service that is not running now but
// can be started.
type unreachableAuthority struct{}

func (unreachableAuthority) Read(context.Context, string, string) ([]byte, uint64, error) {
	return nil, 0, errors.New("connect approval authority: no such file or directory")
}

func (unreachableAuthority) CompareAndSwap(context.Context, string, string, uint64, []byte) error {
	return errors.New("connect approval authority: no such file or directory")
}

var (
	manual = func() bool { return false }
	yolo   = func() bool { return true }
)

// captureDeletionGateNotes points the gate's note at a buffer for one test and
// reports that no terminal interface owns the terminal.
func captureDeletionGateNotes(t *testing.T) *bytes.Buffer {
	t.Helper()
	var notes bytes.Buffer
	previousWriter, previousTerminalUI := deletionGateNotes, deletionGateTerminalUI
	deletionGateNotes, deletionGateTerminalUI = &notes, func() bool { return false }
	t.Cleanup(func() { deletionGateNotes, deletionGateTerminalUI = previousWriter, previousTerminalUI })
	return &notes
}

// runGuardedCall sends one tool call through the gate in the given mode and
// reports whether the command reached the tool.
func runGuardedCall(t *testing.T, guard *deletionguard.Guard, yolo func() bool, call fantasy.ToolCall) (fantasy.ToolResponse, bool) {
	t.Helper()
	ran := false
	inner := fantasy.NewAgentTool(call.Name, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		ran = true
		return fantasy.NewTextResponse("ran"), nil
	})
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, t.TempDir(), yolo)[0]
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	response, err := wrapped.Run(ctx, call)
	require.NoError(t, err)
	return response, ran
}

func TestJobOutputSurvivesUnavailableApprovalAuthority(t *testing.T) {
	for _, toolName := range []string{tools.JobOutputToolName, tools.JobKillToolName} {
		t.Run(toolName, func(t *testing.T) {
			guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
			inner := fantasy.NewAgentTool(toolName, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("job result retained"), nil
			})
			wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, t.TempDir(), manual)[0]
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
			response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: "read", Name: toolName, Input: `{"shell_id":"001"}`})
			require.NoError(t, err, "a reconciliation failure must not fail the agent run")
			require.False(t, response.IsError)
			require.Contains(t, response.Content, "job result retained")
		})
	}
}

// TestDestructiveCommandFollowsTheSelectedMode is the rule the gate is built
// on: the mode authorizes, and the platform never does. Under yolo the gate
// steps aside and notes it. In manual both confirmations stand, and where the
// approval service cannot run the denial names quarantine, never a way round.
func TestDestructiveCommandFollowsTheSelectedMode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		yolo      func() bool
		authority deletionguard.Authority
		runs      bool
		contains  []string
		absent    []string
	}{
		{
			name:      "yolo with a working approval service",
			yolo:      yolo,
			authority: &deletionFixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}},
			runs:      true,
		},
		{name: "yolo where the approval service cannot run", yolo: yolo, authority: unavailableAuthority{}, runs: true},
		{name: "yolo with the approval service stopped", yolo: yolo, authority: unreachableAuthority{}, runs: true},
		{
			name:      "manual with the approval service stopped",
			authority: unreachableAuthority{},
			contains:  []string{"Permanent deletion is blocked", "start the administrator-owned approval service"},
		},
		{
			name:      "manual where the approval service cannot run",
			authority: unavailableAuthority{},
			contains: []string{
				"Permanent deletion is blocked",
				"permanent deletion is unavailable on this platform",
				"Move the items to a quarantine directory instead",
			},
			absent: []string{"start the administrator-owned approval service", "during background reconciliation"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode := tc.yolo
			if mode == nil {
				mode = manual
			}
			guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, tc.authority)
			notes := captureDeletionGateNotes(t)
			response, ran := runGuardedCall(t, guard, mode, fantasy.ToolCall{
				ID: "delete", Name: tools.BashToolName, Input: `{"command":"rm -rf /tmp/ouroboros-fixture"}`,
			})
			require.Equal(t, tc.runs, ran, "whether the command reaches the tool")
			require.Equal(t, !tc.runs, response.IsError, response.Content)
			for _, want := range tc.contains {
				require.Contains(t, response.Content, want)
			}
			for _, absent := range tc.absent {
				require.NotContains(t, response.Content, absent)
			}
			if tc.runs {
				require.Equal(t, 1, strings.Count(notes.String(), "\n"), "one skipped check leaves one note line")
			} else {
				require.Empty(t, notes.String(), "nothing was skipped, so nothing is noted")
			}
		})
	}
}

// TestParallelOverlapRefusalFollowsTheSelectedMode: two concurrent removals
// of the same tree are refused before anything runs in manual. Under yolo the
// whole gate steps aside, so they pass like any other gated command.
func TestParallelOverlapRefusalFollowsTheSelectedMode(t *testing.T) {
	input := `{"commands":[{"command":"rm -rf /tmp/a"},{"command":"rm -rf /tmp/a/child"}]}`
	call := fantasy.ToolCall{ID: "overlap", Name: tools.ParallelBashToolName, Input: input}

	t.Run("manual", func(t *testing.T) {
		guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, &deletionFixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}})
		notes := captureDeletionGateNotes(t)
		response, ran := runGuardedCall(t, guard, manual, call)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "overlapping targets")
		require.False(t, ran)
		require.Empty(t, notes.String())
	})
	t.Run("yolo", func(t *testing.T) {
		guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
		notes := captureDeletionGateNotes(t)
		response, ran := runGuardedCall(t, guard, yolo, call)
		require.False(t, response.IsError, response.Content)
		require.True(t, ran)
		require.Equal(t, 1, strings.Count(notes.String(), "\n"))
	})
}

// TestDefaultRuntimeGateFollowsTheSelectedMode uses the guard the runtime
// builds rather than a fixture.
func TestDefaultRuntimeGateFollowsTheSelectedMode(t *testing.T) {
	call := fantasy.ToolCall{ID: "delete", Name: tools.BashToolName, Input: `{"command":"rm -rf /tmp/ouroboros-fixture"}`}
	t.Run("manual", func(t *testing.T) {
		notes := captureDeletionGateNotes(t)
		response, ran := runGuardedCall(t, deletionguard.NewGuard(t.TempDir(), nil), manual, call)
		require.True(t, response.IsError)
		require.Contains(t, response.Content, "Permanent deletion is blocked")
		require.False(t, ran)
		require.Empty(t, notes.String())
	})
	t.Run("yolo", func(t *testing.T) {
		notes := captureDeletionGateNotes(t)
		response, ran := runGuardedCall(t, deletionguard.NewGuard(t.TempDir(), nil), yolo, call)
		require.False(t, response.IsError, response.Content)
		require.True(t, ran)
		require.Equal(t, 1, strings.Count(notes.String(), "\n"))
	})
}
