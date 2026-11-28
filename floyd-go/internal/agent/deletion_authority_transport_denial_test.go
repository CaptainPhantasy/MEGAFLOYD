package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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

// A platform without an authenticated approval transport has no way to read or
// record a confirmation. The Bash gate steps aside there and notes it; the
// tests below cover the note, what it must not excuse, and the guard itself.

// TestUnavailableApprovalAuthorityCheckItselfFailsClosed: a caller that asks
// the guard directly has no gate to step aside from. Its denial names the real
// remedy rather than telling the reader to start a service this platform cannot
// run.
func TestUnavailableApprovalAuthorityCheckItselfFailsClosed(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	denied := guard.Check(t.Context(), "s", "delete", deletionguard.Classify("rm -rf /tmp/fixture", t.TempDir()))
	require.False(t, denied.Allowed)
	require.Contains(t, denied.Reason, "Permanent deletion is blocked")
	require.Contains(t, denied.Reason, "the protected approval service has no authenticated transport here")
	require.Contains(t, denied.Reason, "quarantine")
	require.NotContains(t, denied.Reason, "start the administrator-owned approval service")
}

func recordingBashTool(ran *bool) fantasy.AgentTool {
	return fantasy.NewAgentTool(tools.BashToolName, "fixture", func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		*ran = true
		return fantasy.NewTextResponse("ran"), nil
	})
}

func runGuardedBash(t *testing.T, guard *deletionguard.Guard, toolCallID, name, input string) (fantasy.ToolResponse, bool) {
	t.Helper()
	ran := false
	wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{recordingBashTool(&ran)}, guard, t.TempDir())[0]
	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
	response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: toolCallID, Name: name, Input: input})
	require.NoError(t, err)
	return response, ran
}

// TestUnavailableApprovalAuthorityDoesNotNoteHarmlessCalls: a command the gate
// never needed to check leaves no note.
func TestUnavailableApprovalAuthorityDoesNotNoteHarmlessCalls(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	for i, command := range []string{`echo ok`, `git -C /tmp status --short`, `rm /tmp/fixture`, `mkdir -p /tmp/q && mv /tmp/a /tmp/b /tmp/q/`} {
		input, err := json.Marshal(map[string]string{"command": command})
		require.NoError(t, err)
		response, ran := runGuardedBash(t, guard, "harmless-"+strconv.Itoa(i), tools.BashToolName, string(input))
		require.False(t, response.IsError, response.Content)
		require.True(t, ran, command)
	}
	require.Empty(t, notes.String())
}

// TestUnavailableApprovalAuthorityNoteStaysOnOneLine: a multi-line command and
// a long one each still produce exactly one bounded line.
func TestUnavailableApprovalAuthorityNoteStaysOnOneLine(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	command := "cat <<'EOF' > /tmp/fixture\nline one\nline two\nEOF\nrm -rf /tmp/" + strings.Repeat("x", 300)
	input, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	response, ran := runGuardedBash(t, guard, "multiline", tools.BashToolName, string(input))
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)

	note := notes.String()
	require.Equal(t, 1, strings.Count(note, "\n"), "one skipped check leaves one line: %q", note)
	require.True(t, strings.HasPrefix(note, "megafloyd: skipped the two-confirmation deletion check. "), note)
	require.Contains(t, note, "so the selected permission mode alone applied to: cat <<'EOF' > /tmp/fixture line one line two EOF rm -rf /tmp/")
	require.True(t, strings.HasSuffix(note, "...\n"), "a long command is cut short: %q", note)
}

// TestUnavailableApprovalAuthorityNotesEveryCommandOfParallelCall: a parallel
// call names each command it covered.
func TestUnavailableApprovalAuthorityNotesEveryCommandOfParallelCall(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	input := `{"commands":[{"command":"rm -rf /tmp/a"},{"command":"rm -rf /tmp/b"}]}`
	response, ran := runGuardedBash(t, guard, "parallel", tools.ParallelBashToolName, input)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Equal(t, 1, strings.Count(notes.String(), "\n"))
	require.Contains(t, notes.String(), "rm -rf /tmp/a; rm -rf /tmp/b")
}

// TestUnavailableApprovalAuthorityStillDeniesOverlappingParallelDeletions: two
// concurrent removals of the same tree are refused before anything is skipped.
func TestUnavailableApprovalAuthorityStillDeniesOverlappingParallelDeletions(t *testing.T) {
	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	notes := captureDeletionGateNotes(t)
	input := `{"commands":[{"command":"rm -rf /tmp/a"},{"command":"rm -rf /tmp/a/child"}]}`
	response, ran := runGuardedBash(t, guard, "overlap", tools.ParallelBashToolName, input)
	require.True(t, response.IsError)
	require.Contains(t, response.Content, "overlapping targets")
	require.False(t, ran)
	require.Empty(t, notes.String())
}

// TestUnavailableApprovalAuthorityKeepsTerminalDisplayClean: while the
// terminal interface is active, process stderr is the screen itself. The note
// goes to the application log and nothing is written to the screen.
func TestUnavailableApprovalAuthorityKeepsTerminalDisplayClean(t *testing.T) {
	var screen bytes.Buffer
	previousWriter, previousTerminal := deletionGateNotes, deletionGateTerminal
	deletionGateNotes, deletionGateTerminal = &screen, func(writer io.Writer) bool { return true }
	t.Cleanup(func() { deletionGateNotes, deletionGateTerminal = previousWriter, previousTerminal })

	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	guard := deletionguard.NewGuardWithAuthority(t.TempDir(), nil, unavailableAuthority{})
	input := `{"command":"rm -rf /tmp/fixture"}`
	response, ran := runGuardedBash(t, guard, "terminal", tools.BashToolName, input)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Empty(t, screen.String(), "nothing may be written to the terminal")
	require.Contains(t, logged.String(), "Deletion check skipped")
	require.Contains(t, logged.String(), "session_id=s")
	require.Contains(t, logged.String(), "rm -rf /tmp/fixture")
}

// TestDefaultRuntimeGateStepsAsideWhereNoServiceCanRun uses the guard the
// runtime builds rather than a fixture.
func TestDefaultRuntimeGateStepsAsideWhereNoServiceCanRun(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux has an authenticated transport, so its gate needs the service fixture")
	}
	guard := deletionguard.NewGuard(t.TempDir(), nil)
	notes := captureDeletionGateNotes(t)
	input := `{"command":"rm -rf /tmp/fixture"}`
	response, ran := runGuardedBash(t, guard, "runtime", tools.BashToolName, input)
	require.False(t, response.IsError, response.Content)
	require.True(t, ran)
	require.Equal(t, 1, strings.Count(notes.String(), "\n"))
}

// yoloStep is one command a yolo session sent while the Bash gate refused it,
// what it printed, and what it must have left behind.
type yoloStep struct {
	name, command, prints string
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
		{"git init", "git init -q " + repo, "", exists(repo)},
		{"write a file", `printf 'host = "192.168.1.50"\n' > ` + repo + "/net.py && echo write-ok", "write-ok", exists(filepath.Join(repo, "net.py"))},
		{"git -C add", "git -C " + repo + " add net.py && echo add-alone-ok", "add-alone-ok", nil},
		{"git -C status", "git -C " + repo + " status --short && echo status-c-ok", "A  net.py", nil},
		{"redirect after cd", "cd " + repo + " && echo test > b.txt && echo ok2", "ok2", func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repo, "b.txt"))
			require.NoError(t, err)
			require.Equal(t, "test\n", string(content))
		}},
		{"plain rm", "rm " + verdicts, "", gone(verdicts)},
		{"rm -r", "rm -r " + scratch, "", gone(scratch)},
		{
			"mv to quarantine", "mkdir -p " + quarantine + " && mv " + repo + " " + verdicts2 + " " + gitTest + " " + quarantine +
				"/ 2>/dev/null; ls " + quarantine + " | grep -i gap; echo moved", "moved",
			exists(filepath.Join(quarantine, "gaptest-repo", "net.py"), filepath.Join(quarantine, "gap_verdicts2.py"), filepath.Join(quarantine, "gap_git_test.py")),
		},
	}
}

// TestYoloSessionCommandsRunThroughRealBashTool replays what the owner's yolo
// session sent, through the real Bash tool in the mode `megafloyd -y` selects.
// No step may be refused or ask a question. A forced removal is the one
// command the gate still has a say in.
func TestYoloSessionCommandsRunThroughRealBashTool(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	for _, runtimeGuard := range []bool{false, true} {
		name := map[bool]string{false: "working approval service", true: "runtime guard on this platform"}[runtimeGuard]
		t.Run(name, func(t *testing.T) {
			if runtimeGuard && runtime.GOOS == "linux" {
				t.Skip("Linux has an authenticated transport, so its gate needs the service fixture")
			}
			f := newDeletionFixture(t)
			guard := f.guard
			if runtimeGuard {
				guard = deletionguard.NewGuard(t.TempDir(), nil)
			}
			f.answers = []bool{true, true}
			for _, name := range []string{"gap_verdicts.py", "gap_verdicts2.py", "gap_git_test.py"} {
				require.NoError(t, os.WriteFile(f.target(name), []byte("fixture"), 0o600))
			}
			require.NoError(t, os.MkdirAll(f.target("gaptest-scratch"), 0o700))

			// The second argument, true, is what `megafloyd -y` selects.
			inner := tools.NewBashTool(permission.NewPermissionService(f.workingDir, true, nil), f.workingDir, &config.Attribution{TrailerStyle: config.TrailerStyleNone}, "fixture")
			wrapped := wrapToolsWithDeletionGuard([]fantasy.AgentTool{inner}, guard, f.workingDir)[0]
			ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "s")
			notes := captureDeletionGateNotes(t)
			run := func(id, command string) fantasy.ToolResponse {
				t.Helper()
				input, err := json.Marshal(map[string]string{"command": command})
				require.NoError(t, err)
				response, err := wrapped.Run(ctx, fantasy.ToolCall{ID: id, Name: tools.BashToolName, Input: string(input)})
				require.NoError(t, err)
				require.False(t, response.IsError, "%s: %s", command, response.Content)
				return response
			}

			for i, step := range yoloSteps(f.workingDir) {
				response := run("yolo-"+strconv.Itoa(i), step.command)
				if step.prints != "" {
					require.Contains(t, response.Content, step.prints, step.name)
				}
				if step.leaves != nil {
					step.leaves(t)
				}
			}
			// The cleanup command that session tried last.
			cleanup := "printf x > gap_left.py; rm gap_left.py; ls " + f.workingDir + " | grep -i gap; echo cleanup-done"
			require.Contains(t, run("yolo-cleanup", cleanup).Content, "cleanup-done")
			require.Empty(t, f.asked, "no ordinary command may ask for confirmation")
			require.Empty(t, notes.String(), "no ordinary command is the gate's business")

			run("yolo-forced", "rm -rf "+filepath.Join(f.workingDir, "quarantine-gap-fixtures"))
			_, err := os.Lstat(filepath.Join(f.workingDir, "quarantine-gap-fixtures"))
			require.ErrorIs(t, err, os.ErrNotExist)
			if runtimeGuard {
				require.Empty(t, f.asked)
				require.Equal(t, 1, strings.Count(notes.String(), "\n"), "only the forced removal is noted")
			} else {
				require.Len(t, f.asked, 2, "a working approval service still asks twice for a forced removal")
				require.Empty(t, notes.String())
			}
		})
	}
}
