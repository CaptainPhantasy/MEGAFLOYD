package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/agent/tools"
	"github.com/legacy-ai/floyd/internal/config"
	"github.com/legacy-ai/floyd/internal/deletionguard"
	"github.com/legacy-ai/floyd/internal/permission"
	"github.com/stretchr/testify/require"
)

// The selected permission mode decides whether the Bash gate runs. Under yolo
// it steps aside and leaves a note. The tests below cover what that note may
// contain, where it goes, the guard asked directly, and the session whose
// blocked commands started the repair.

// TestUnavailableApprovalAuthorityCheckItselfFailsClosed: a caller that asks
// the guard directly has no gate to step aside from. Its denial names the real
// remedy rather than telling the reader to start a service this platform cannot
// run.
func TestUnavailableApprovalAuthorityCheckItselfFailsClosed(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	denied := guard.Check(t.Context(), "s", "delete", deletionguard.Classify("rm -rf "+filepath.Join(t.TempDir(), "fixture"), t.TempDir()))
	require.False(t, denied.Allowed)
	require.Contains(t, denied.Reason, "Permanent deletion is blocked")
	require.Contains(t, denied.Reason, "the protected approval service has no authenticated transport here")
	require.Contains(t, denied.Reason, "quarantine")
	require.NotContains(t, denied.Reason, "start the administrator-owned approval service")
}

// bashCall builds a Bash tool call for one command.
func bashCall(t *testing.T, id, command string) fantasy.ToolCall {
	t.Helper()
	input, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	return fantasy.ToolCall{ID: id, Name: tools.BashToolName, Input: string(input)}
}

// TestSkippedCheckNoteCarriesNoCommandText: a command can hold a credential,
// and the note is written before the permission check has looked at the call.
// It names the session, the tool call and a count, nothing else.
func TestSkippedCheckNoteCarriesNoCommandText(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	command := "API_TOKEN=do-not-print-this some-client --token do-not-print-this; rm -rf /tmp/fixture"
	response, ran := runGuardedCall(t, guard, yolo, bashCall(t, "secret", command))
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)

	note := notes.String()
	require.Equal(t, 1, strings.Count(note, "\n"), "one skipped check leaves one line: %q", note)
	for _, want := range []string{"skipped the two-confirmation deletion check for 1 command(s)", "Yolo is selected", "(session s, tool call secret)"} {
		require.Contains(t, note, want)
	}
	for _, text := range []string{"do-not-print-this", "some-client", "rm -rf", "API_TOKEN", "fixture"} {
		require.NotContains(t, note, text)
	}
}

// TestSkippedCheckNoteCountsTheCommandsOfAParallelCall: a parallel call is
// counted, not quoted.
func TestSkippedCheckNoteCountsTheCommandsOfAParallelCall(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	call := fantasy.ToolCall{
		ID: "parallel", Name: tools.ParallelBashToolName,
		Input: `{"commands":[{"command":"rm -rf /tmp/first-private"},{"command":"rm -rf /tmp/second-private"}]}`,
	}
	response, ran := runGuardedCall(t, guard, yolo, call)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)

	note := notes.String()
	require.Equal(t, 1, strings.Count(note, "\n"))
	require.Contains(t, note, "for 2 command(s)")
	for _, text := range []string{"first-private", "second-private", "rm -rf"} {
		require.NotContains(t, note, text)
	}
}

// TestSkippedCheckNoteGoesToTheLogWhileTheTerminalUIOwnsTheTerminal: the
// terminal interface paints the screen from its own buffer, so a line written
// to the terminal behind its back would stay there. While it is active the note
// goes to the application log instead, still without command text.
func TestSkippedCheckNoteGoesToTheLogWhileTheTerminalUIOwnsTheTerminal(t *testing.T) {
	var screen bytes.Buffer
	previousWriter, previousTerminalUI := deletionGateNotes, deletionGateTerminalUI
	deletionGateNotes, deletionGateTerminalUI = &screen, func() bool { return true }
	t.Cleanup(func() { deletionGateNotes, deletionGateTerminalUI = previousWriter, previousTerminalUI })

	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	response, ran := runGuardedCall(t, guard, yolo, bashCall(t, "terminal", "rm -rf /tmp/private-command"))
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)

	require.Empty(t, screen.String(), "nothing may be written to the terminal")
	for _, want := range []string{"Deletion check skipped", "session_id=s ", "tool_call_id=terminal", "command_count=1"} {
		require.Contains(t, logged.String(), want)
	}
	require.NotContains(t, logged.String(), "private-command")
}

// TestSkippedCheckNoteReachesProcessStderrWithoutTheTerminalUI: a
// non-interactive run may be attached to a terminal and still gets its note on
// stderr. Routing follows the terminal interface, not the kind of writer.
func TestSkippedCheckNoteReachesProcessStderrWithoutTheTerminalUI(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	previousWriter, previousTerminalUI := deletionGateNotes, deletionGateTerminalUI
	deletionGateNotes, deletionGateTerminalUI = writer, func() bool { return false }
	t.Cleanup(func() { deletionGateNotes, deletionGateTerminalUI = previousWriter, previousTerminalUI })

	var written bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = written.ReadFrom(reader)
		close(done)
	}()

	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	_, ran := runGuardedCall(t, guard, yolo, bashCall(t, "piped", "rm -rf /tmp/fixture"))
	require.NoError(t, writer.Close())
	<-done
	require.True(t, ran)
	require.Equal(t, 1, strings.Count(written.String(), "\n"), "%q", written.String())
	require.Contains(t, written.String(), "skipped the two-confirmation deletion check for 1 command(s)")
}

// TestGateLeavesNoNoteForCommandsItNeverChecks: a command that removes nothing
// is not the gate's business in any mode.
func TestGateLeavesNoNoteForCommandsItNeverChecks(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.WriteFile(filepath.Join(base, name), []byte(name), 0o600))
	}
	for _, mode := range []func() bool{manual, yolo} {
		guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
		notes := captureDeletionGateNotes(t)
		for i, command := range []string{
			"echo ok",
			"git -C " + base + " status --short",
			"mkdir -p " + filepath.Join(base, "q") + " && mv " + filepath.Join(base, "a") + " " + filepath.Join(base, "b") + " " + filepath.Join(base, "q") + "/",
		} {
			response, ran := runGuardedCall(t, guard, mode, bashCall(t, "harmless-"+strconv.Itoa(i), command))
			require.False(t, response.IsError, response.Content)
			require.True(t, ran, command)
		}
		require.Empty(t, notes.String())
	}
}

// TestGateReadsTheSelectedModeOnEveryCall: the owner can change the mode
// between calls, so the gate asks the permission service each time instead of
// deciding once when the tools are built.
func TestGateReadsTheSelectedModeOnEveryCall(t *testing.T) {
	service := permission.NewPermissionService(t.TempDir(), false, nil)
	mode := service.(permission.ModeService)
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, &deletionFixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}})
	notes := captureDeletionGateNotes(t)
	call := bashCall(t, "delete", "rm -rf "+filepath.Join(t.TempDir(), "fixture"))
	send := func(wantRan bool) {
		t.Helper()
		before := strings.Count(notes.String(), "\n")
		response, ran := runGuardedCall(t, guard, func() bool { return permission.Yolo(service) }, call)
		require.Equal(t, wantRan, ran, response.Content)
		if wantRan {
			require.Equal(t, before+1, strings.Count(notes.String(), "\n"))
		} else {
			require.Contains(t, response.Content, "Permanent deletion")
			require.Equal(t, before, strings.Count(notes.String(), "\n"))
		}
	}
	send(false)
	mode.SetMode(permission.ModeYolo)
	send(true)
	mode.SetMode(permission.ModeManual)
	send(false)
	service.SetSkipRequests(true)
	send(true)
	mode.SetMode(permission.ModeManual)
	send(false)
}

// yoloStep is one command the yolo session sent while the Bash gate refused it,
// what it printed, and what it must have left behind.
type yoloStep struct {
	name, command, prints string
	removal               bool
	leaves                func(t *testing.T)
}

func yoloSteps(base string) []yoloStep {
	repo := filepath.Join(base, "gaptest-repo")
	verdicts := filepath.Join(base, "gap_verdicts.py")
	verdicts2 := filepath.Join(base, "gap_verdicts2.py")
	gitTest := filepath.Join(base, "gap_git_test.py")
	scratch := filepath.Join(base, "gaptest-scratch")
	quarantine := filepath.Join(base, "quarantine-gap-fixtures")
	exists := func(paths ...string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			for _, path := range paths {
				_, err := os.Stat(path)
				require.NoError(t, err, path)
			}
		}
	}
	gone := func(paths ...string) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()
			for _, path := range paths {
				_, err := os.Lstat(path)
				require.ErrorIs(t, err, os.ErrNotExist, path)
			}
		}
	}
	return []yoloStep{
		{"git init", "git init -q " + repo, "", false, exists(repo)},
		// 192.0.2.10 is from the documentation range reserved for examples.
		{"write a file", `printf 'host = "192.0.2.10"\n' > ` + repo + "/net.py && echo write-ok", "write-ok", false, exists(filepath.Join(repo, "net.py"))},
		{"git -C add", "git -C " + repo + " add net.py && echo add-alone-ok", "add-alone-ok", false, nil},
		{"git -C status", "git -C " + repo + " status --short && echo status-c-ok", "A  net.py", false, nil},
		{"redirect after cd", "cd " + repo + " && echo test > b.txt && echo ok2", "ok2", false, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repo, "b.txt"))
			require.NoError(t, err)
			require.Equal(t, "test\n", string(content))
		}},
		{"plain rm", "rm " + verdicts, "", true, gone(verdicts)},
		{"rm -r", "rm -r " + scratch, "", true, gone(scratch)},
		{
			"mv to quarantine", "mkdir -p " + quarantine + " && mv " + repo + " " + verdicts2 + " " + gitTest + " " + quarantine +
				"/ 2>/dev/null; ls " + quarantine + " | grep -i gap; echo moved", "moved", false,
			exists(filepath.Join(quarantine, "gaptest-repo", "net.py"), filepath.Join(quarantine, "gap_verdicts2.py"), filepath.Join(quarantine, "gap_git_test.py")),
		},
	}
}

// yoloReplayFixture lays out the files the yolo session used. The repository
// directory exists from the start, so the commands also classify correctly
// when nothing is executed.
func yoloReplayFixture(t *testing.T, base string) {
	t.Helper()
	for _, name := range []string{"gap_verdicts.py", "gap_verdicts2.py", "gap_git_test.py"} {
		require.NoError(t, os.WriteFile(filepath.Join(base, name), []byte("fixture"), 0o600))
	}
	for _, name := range []string{"gaptest-repo", "gaptest-scratch"} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, name), 0o700))
	}
}

// TestYoloSessionCommandsRunThroughRealBashTool replays what the owner's yolo
// session sent, through the real Bash tool in the mode `megafloyd -y` selects.
// No step may be refused or ask a question. Each removal is the gate's
// business, and under yolo the gate steps aside and notes it; the three
// misclassified commands never were.
func TestYoloSessionCommandsRunThroughRealBashTool(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	for _, runtimeGuard := range []bool{false, true} {
		name := map[bool]string{false: "working approval service", true: "runtime guard on this platform"}[runtimeGuard]
		t.Run(name, func(t *testing.T) {
			f := newDeletionFixture(t)
			base := f.workingDir
			yoloReplayFixture(t, base)
			guard := f.guard
			if runtimeGuard {
				guard = deletionguard.NewGuard(t.TempDir(), nil)
			}
			// Skip requests is what `megafloyd -y` selects.
			permissions := permission.NewPermissionService(base, true, nil)
			inner := tools.NewBashTool(permissions, base, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
			wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, base, func() bool { return permission.Yolo(permissions) })[0]
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
			notes := captureDeletionGateNotes(t)
			run := func(id, command string) fantasy.ToolResponse {
				t.Helper()
				response, err := wrapped.Run(ctx, bashCall(t, id, command))
				require.NoError(t, err)
				require.False(t, response.IsError, "%s: %s", command, response.Content)
				return response
			}

			gated := 0
			for i, step := range yoloSteps(base) {
				response := run("yolo-"+strconv.Itoa(i), step.command)
				if step.prints != "" {
					require.Contains(t, response.Content, step.prints, step.name)
				}
				if step.leaves != nil {
					step.leaves(t)
				}
				if step.removal {
					gated++
				}
			}
			// The cleanup command that session tried last.
			left := filepath.Join(base, "gap_left.py")
			require.NoError(t, os.WriteFile(left, []byte("x"), 0o600))
			cleanup := "rm " + left + "; ls " + base + " | grep -i gap; echo cleanup-done"
			require.Contains(t, run("yolo-cleanup", cleanup).Content, "cleanup-done")
			gated++
			_, err := os.Lstat(left)
			require.ErrorIs(t, err, os.ErrNotExist)

			quarantine := filepath.Join(base, "quarantine-gap-fixtures")
			run("yolo-forced", "rm -rf "+quarantine)
			gated++
			_, err = os.Lstat(quarantine)
			require.ErrorIs(t, err, os.ErrNotExist)

			// Every gated command was noted once, and nothing was ever asked.
			require.Equal(t, gated, strings.Count(notes.String(), "\n"))
			require.Empty(t, f.asked, "yolo never asks for a confirmation")
			for _, text := range []string{"rm ", "mv ", "git ", "192.0.2.10", "gap_"} {
				require.NotContains(t, notes.String(), text)
			}
		})
	}
}

// TestManualModeKeepsTheGateForTheSameCommands: the session's commands that
// remove nothing run in manual too, and each removal is refused until a human
// answers twice. A platform without an approval service changes the reason,
// not the outcome.
func TestManualModeKeepsTheGateForTheSameCommands(t *testing.T) {
	for _, tc := range []struct {
		name      string
		authority deletionguard.Authority
		refusal   string
		asked     int
	}{
		{
			name:      "working approval service",
			authority: &deletionFixtureAuthority{data: map[string][]byte{}, revisions: map[string]uint64{}},
			refusal:   "not confirmed at stage 1 of 2",
			// Each refused command asks its first question once.
			asked: 2,
		},
		{
			name: "where the approval service cannot run",
			// Nothing can be recorded, so nothing is asked.
			authority: unavailableAuthority{},
			refusal:   "Move the items to a quarantine directory instead",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeletionFixture(t)
			base := f.workingDir
			yoloReplayFixture(t, base)
			guard := deletionguard.NewGuardWithAuthority(f.dataDir, f, tc.authority)
			notes := captureDeletionGateNotes(t)
			passed, refused := 0, 0
			for i, step := range yoloSteps(base) {
				response, ran := runGuardedCall(t, guard, manual, bashCall(t, "manual-"+strconv.Itoa(i), step.command))
				if !step.removal {
					require.True(t, ran, step.name)
					passed++
					continue
				}
				require.False(t, ran, step.name)
				require.Contains(t, response.Content, tc.refusal, step.name)
				refused++
			}
			steps := yoloSteps(base)
			require.Equal(t, 2, refused, "the two rm commands are refused")
			require.Equal(t, len(steps)-2, passed, "every other command reaches the tool")
			require.Empty(t, notes.String(), "nothing is skipped in manual")
			require.Len(t, f.asked, tc.asked)
		})
	}
}
