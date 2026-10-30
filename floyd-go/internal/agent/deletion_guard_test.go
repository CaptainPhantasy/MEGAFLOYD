package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/stretchr/testify/require"
)

// deletionRecordingTool stands in for the bash tool body. It records whether
// the guarded command reached the tool at all, and never runs a command.
type deletionRecordingTool struct {
	ran  *bool
	resp fantasy.ToolResponse
	err  error
}

func (t deletionRecordingTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{Name: tools.BashToolName, Description: "test bash"}
}

func (t deletionRecordingTool) ProviderOptions() fantasy.ProviderOptions {
	return fantasy.ProviderOptions{}
}

func (t deletionRecordingTool) SetProviderOptions(fantasy.ProviderOptions) {}

func (t deletionRecordingTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	*t.ran = true
	return t.resp, t.err
}

// deletionFixture holds one disposable working directory, one data directory
// for the session record, and the scripted human answers.
type deletionFixture struct {
	t          *testing.T
	workingDir string
	dataDir    string
	guard      *deletionguard.Guard
	answers    []bool
	asked      []deletionguard.ConfirmationRequest
	calls      int
}

func newDeletionFixture(t *testing.T) *deletionFixture {
	t.Helper()
	f := &deletionFixture{t: t, workingDir: t.TempDir(), dataDir: t.TempDir()}
	f.guard = deletionguard.NewGuard(f.dataDir, f)
	return f
}

// Confirm implements deletionguard.Confirmer, answering from the script.
func (f *deletionFixture) Confirm(_ context.Context, req deletionguard.ConfirmationRequest) (bool, error) {
	f.asked = append(f.asked, req)
	if len(f.answers) == 0 {
		return false, nil
	}
	answer := f.answers[0]
	f.answers = f.answers[1:]
	return answer, nil
}

// run sends one bash tool call through the deletion guard.
func (f *deletionFixture) run(sessionID, command string) (fantasy.ToolResponse, bool) {
	f.t.Helper()
	ran := false
	exitCode := 0
	inner := deletionRecordingTool{ran: &ran, resp: fantasy.WithResponseMetadata(fantasy.NewTextResponse("ran"), tools.BashResponseMetadata{ExitCode: &exitCode})}
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, f.guard, f.workingDir)[0]
	f.calls++
	input, err := json.Marshal(map[string]string{"command": command})
	require.NoError(f.t, err)
	ctx := context.WithValue(f.t.Context(), tools.SessionIDContextKey, sessionID)
	response, err := wrapped.Run(ctx, fantasy.ToolCall{
		ID:    "deletion-call-" + strconv.Itoa(f.calls),
		Name:  tools.BashToolName,
		Input: string(input),
	})
	require.NoError(f.t, err)
	return response, ran
}

// confirmBoth writes the two human confirmations for an exact target set, as
// the gate would after asking.
func (f *deletionFixture) confirmBoth(sessionID string, targets ...string) {
	f.t.Helper()
	targetSet := deletionguard.Digest(targets)
	for stage := 1; stage <= 2; stage++ {
		require.NoError(f.t, f.guard.Ledger().AppendConfirmation(sessionID, deletionguard.Confirmation{
			Stage: stage, TargetSet: targetSet, Targets: targets, Source: "human",
		}))
	}
}

func (f *deletionFixture) record(sessionID string) deletionguard.Record {
	f.t.Helper()
	record, err := f.guard.Ledger().Read(sessionID)
	require.NoError(f.t, err)
	return record
}

func (f *deletionFixture) target(name string) string {
	return filepath.Join(f.workingDir, name)
}

func TestDeletionGuardRequiresTwoConfirmations(t *testing.T) {
	f := newDeletionFixture(t)

	response, ran := f.run("session-deletion", "rm -rf widget")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran, "the command must not reach the tool body")
	require.Len(t, f.asked, 1)
	require.Equal(t, 1, f.asked[0].Stage)
	require.Empty(t, f.record("session-deletion").Confirmations)
}

func TestDeletionGuardSingleConfirmationInsufficient(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, false}

	response, ran := f.run("session-deletion", "rm -rf widget")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran)
	require.Len(t, f.asked, 2)
	record := f.record("session-deletion")
	require.Len(t, record.Confirmations, 1, "only the first stage was confirmed")
	require.Equal(t, 1, record.Confirmations[0].Stage)

	// The recorded first answer is reused; a later attempt asks only stage two.
	f.answers = []bool{true}
	response, ran = f.run("session-deletion", "rm -rf widget")
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Len(t, f.asked, 3)
	require.Equal(t, 2, f.asked[2].Stage)
}

func TestDeletionGuardAllowsAfterTwoConfirmations(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}

	response, ran := f.run("session-deletion", "rm -rf widget")

	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Len(t, f.asked, 2)
	record := f.record("session-deletion")
	require.Len(t, record.Confirmations, 2)
	for _, confirmation := range record.Confirmations {
		require.Equal(t, "human", confirmation.Source)
		require.Equal(t, deletionguard.Digest([]string{f.target("widget")}), confirmation.TargetSet)
		require.Equal(t, []string{f.target("widget")}, confirmation.Targets)
	}
	require.Len(t, record.Executions, 1, "the completed deletion is recorded")
	require.Equal(t, "deletion-call-1", record.Executions[0].ToolCallID)
}

func TestDeletionGuardDoesNotRepeatCompletedDeletion(t *testing.T) {
	f := newDeletionFixture(t)
	f.answers = []bool{true, true}
	_, ran := f.run("session-deletion", "rm -rf widget")
	require.True(t, ran)

	f.answers = []bool{true, true}
	response, ran := f.run("session-deletion", "rm -rf widget")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran, "a completed deletion must not run again")
	require.Len(t, f.asked, 2, "no new confirmation is asked for a completed deletion")
	require.Contains(t, response.Content, "already ran")
	require.Len(t, f.record("session-deletion").Executions, 1)
}

func TestDeletionGuardRejectsAdditionalTarget(t *testing.T) {
	f := newDeletionFixture(t)
	sessionID := "session-deletion"
	f.confirmBoth(sessionID, f.target("widget"))

	// The confirmed set covers only widget; adding logs is not approved.
	response, ran := f.run(sessionID, "rm -rf widget logs")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran)
	require.Equal(t, deletionguard.CanonicalTargets([]string{f.target("widget"), f.target("logs")}), f.asked[0].Targets)
	record := f.record(sessionID)
	require.Len(t, record.Confirmations, 2, "the earlier pair still covers only the old set")
	require.Equal(t, deletionguard.Digest([]string{f.target("widget")}), record.Confirmations[0].TargetSet)
}

func TestDeletionGuardAllowsQuarantineMove(t *testing.T) {
	f := newDeletionFixture(t)

	response, ran := f.run("session-deletion", "mv widget .quarantine/widget")

	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Empty(t, f.asked, "a recoverable move needs no confirmation")
	require.Empty(t, f.record("session-deletion").Confirmations)
}

func TestDeletionGuardFailsClosedOnMalformedRecord(t *testing.T) {
	f := newDeletionFixture(t)
	sessionID := "session-deletion"
	require.NoError(t, os.MkdirAll(f.guard.Ledger().Dir(sessionID), 0o755))
	require.NoError(t, os.WriteFile(f.guard.Ledger().Path(sessionID), []byte("{not json"), 0o644))
	f.answers = []bool{true, true}

	response, ran := f.run(sessionID, "rm -rf widget")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran)
	require.Empty(t, f.asked, "an unreadable record must not be repaired by asking")
	require.Contains(t, response.Content, "cannot be read")
}

func TestDeletionGuardFailsClosedWithoutHumanChannel(t *testing.T) {
	f := newDeletionFixture(t)
	f.guard.SetConfirmer(nil)

	response, ran := f.run("session-deletion", "rm -rf widget")

	require.True(t, response.IsError, response.Content)
	require.False(t, ran)
	require.Contains(t, response.Content, "needs two separate human confirmations")
}

func TestDeletionGuardAllowsNonDestructiveCommand(t *testing.T) {
	f := newDeletionFixture(t)

	response, ran := f.run("session-deletion", "echo hello")

	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Empty(t, f.asked)
}

func TestDeletionGuardWiredThroughCoordinatorTools(t *testing.T) {
	workingDir := t.TempDir()
	cfg := config.NewTestStoreWithWorkingDir(&config.Config{
		Options: &config.Options{DataDirectory: t.TempDir()},
	}, workingDir)
	c := &coordinator{cfg: cfg}

	ran := false
	inner := deletionRecordingTool{ran: &ran, resp: fantasy.NewTextResponse("ran")}
	wrapped := c.wrapAgentTools([]fantasy.AgentTool{inner}, false)
	require.NotNil(t, wrapped)

	input, err := json.Marshal(map[string]string{"command": "rm -rf widget"})
	require.NoError(t, err)
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "session-deletion")
	response, err := wrapped[0].Run(ctx, fantasy.ToolCall{ID: "deletion-wired", Name: tools.BashToolName, Input: string(input)})

	require.NoError(t, err)
	require.True(t, response.IsError, response.Content)
	require.False(t, ran, "the coordinator chain must gate the command before the tool body")
}
