package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/legacy-ai/floyd/internal/megafloyd"
	"github.com/stretchr/testify/require"
)

func fakeGit(output string) gitRunner {
	return func(context.Context, string, ...string) ([]byte, error) {
		return []byte(output), nil
	}
}

func readCheckpoint(t *testing.T, cwd string) Checkpoint {
	t.Helper()
	data, err := os.ReadFile(Path(cwd))
	require.NoError(t, err)
	var cp Checkpoint
	require.NoError(t, json.Unmarshal(data, &cp))
	return cp
}

func TestWriterCreatesCheckpoint(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	ts := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	w := Writer{
		CWD: cwd,
		Now: func() time.Time { return ts },
		git: fakeGit("## main...origin/main\n M file.go\n?? new.txt\n"),
	}

	err := w.Write(t.Context(), Checkpoint{
		SessionID: "sess-123",
		Note:      "working on audit item 16",
	})
	require.NoError(t, err)

	info, err := os.Stat(Path(cwd))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	cp := readCheckpoint(t, cwd)
	require.True(t, cp.Timestamp.Equal(ts))
	require.Equal(t, cwd, cp.CWD)
	require.Equal(t, "sess-123", cp.SessionID)
	require.Equal(t, "working on audit item 16", cp.Note)
	require.NotNil(t, cp.Git)
	require.Equal(t, "main", cp.Git.Branch)
	require.True(t, cp.Git.Dirty)
	require.Equal(t, 2, cp.Git.Changes)
}

func TestWriteLeavesNoTemporaryFile(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	w := Writer{CWD: cwd, git: fakeGit("## main\n")}

	require.NoError(t, w.Write(t.Context(), Checkpoint{SessionID: "first"}))
	require.NoError(t, w.Write(t.Context(), Checkpoint{SessionID: "second"}))

	matches, err := filepath.Glob(filepath.Join(cwd, DirName, FileName+"-*.tmp"))
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestOlderCheckpointCannotClobberNewer(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	newer := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Minute)

	write := func(ts time.Time, sessionID string) error {
		w := Writer{
			CWD: cwd,
			Now: func() time.Time { return ts },
			git: fakeGit("## main\n"),
		}
		return w.Write(t.Context(), Checkpoint{SessionID: sessionID})
	}

	require.NoError(t, write(newer, "newer"))
	require.ErrorIs(t, write(older, "older"), ErrStale)
	require.Equal(t, "newer", readCheckpoint(t, cwd).SessionID)

	require.NoError(t, write(newer.Add(time.Minute), "latest"))
	require.Equal(t, "latest", readCheckpoint(t, cwd).SessionID)
}

func TestWriteTruncatesNoteToSizeLimit(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	note := strings.Repeat("x", 8*MaxSize)
	w := Writer{CWD: cwd, git: fakeGit("## main\n")}

	require.NoError(t, w.Write(t.Context(), Checkpoint{
		SessionID: "sess-123",
		Note:      note,
	}))

	data, err := os.ReadFile(Path(cwd))
	require.NoError(t, err)
	require.LessOrEqual(t, len(data), MaxSize)

	cp := readCheckpoint(t, cwd)
	require.NotEmpty(t, cp.Note)
	require.Less(t, len(cp.Note), len(note))
	require.True(t, strings.HasPrefix(note, cp.Note))
	require.Equal(t, "sess-123", cp.SessionID)
	require.Equal(t, cwd, cp.CWD)
	require.False(t, cp.Timestamp.IsZero())
	require.NotNil(t, cp.Git)
	require.Equal(t, "main", cp.Git.Branch)
}

func TestWriteWithoutGit(t *testing.T) {
	// PATH is process-wide, so this test cannot run in parallel.
	cwd := t.TempDir()
	t.Setenv("PATH", t.TempDir())

	w := Writer{CWD: cwd}
	require.NoError(t, w.Write(t.Context(), Checkpoint{SessionID: "sess-123"}))

	data, err := os.ReadFile(Path(cwd))
	require.NoError(t, err)
	require.NotContains(t, string(data), `"git"`)

	cp := readCheckpoint(t, cwd)
	require.Nil(t, cp.Git)
	require.Equal(t, "sess-123", cp.SessionID)
	require.Equal(t, cwd, cp.CWD)
	require.False(t, cp.Timestamp.IsZero())
}

// initGitRepo creates a repository with one committed file, so a dirty tree
// can only come from the code under test.
func initGitRepo(t *testing.T, gitPath, cwd string) {
	t.Helper()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitPath, append([]string{"-C", cwd}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.invalid",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	runGit("init", "-q", "-b", "main", ".")
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "tracked.txt"), []byte("hi"), 0o644))
	runGit("add", "tracked.txt")
	runGit("-c", "commit.gpgsign=false", "commit", "-qm", "init")
}

func TestCheckpointExcludesFollowsActualDirectory(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	require.Equal(t,
		[]string{".megafloyd/" + FileName, ".megafloyd/" + FileName + "-*.tmp"},
		checkpointExcludes(cwd, filepath.Join(cwd, ".megafloyd")))
	require.Equal(t,
		[]string{".floyd/" + FileName, ".floyd/" + FileName + "-*.tmp"},
		checkpointExcludes(cwd, filepath.Join(cwd, ".floyd")))
	require.Equal(t,
		[]string{FileName, FileName + "-*.tmp"},
		checkpointExcludes(cwd, cwd),
		"a checkpoint written directly into cwd excludes only the files")

	// A checkpoint the working tree never reports needs no exclusion, and an
	// unreadable relationship must not exclude everything.
	require.Nil(t, checkpointExcludes(cwd, filepath.Join(t.TempDir(), "elsewhere")))
	require.Nil(t, checkpointExcludes(cwd, filepath.Join(cwd, "..", "sibling")))
}

func TestWriteExcludesCheckpointInActiveStateDirectory(t *testing.T) {
	// The active identity keeps project state in .megafloyd. The probe must
	// exclude that directory, not the legacy name, or the checkpoint lands in
	// its own summary as a dirty file. The identity is read from the process
	// environment, so this test cannot run in parallel.
	t.Setenv(megafloyd.ActiveEnv, "1")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	cwd := t.TempDir()
	initGitRepo(t, gitPath, cwd)

	stateDir := filepath.Join(cwd, megafloyd.ProjectDirName())
	require.Equal(t, filepath.Join(cwd, ".megafloyd"), stateDir,
		"the active identity must select .megafloyd for this test to mean anything")

	w := Writer{CWD: cwd, DataDir: stateDir}
	// The first write creates the state directory; the second one is the probe
	// that matters, because it runs with a checkpoint already on disk.
	require.NoError(t, w.Write(t.Context(), Checkpoint{}))
	require.NoError(t, w.Write(t.Context(), Checkpoint{}))

	require.Equal(t, filepath.Join(stateDir, FileName), Path(cwd))
	_, err = os.Stat(Path(cwd))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(cwd, ".floyd"))
	require.ErrorIs(t, err, os.ErrNotExist, "no checkpoint may be written to the legacy directory")

	cp := readCheckpoint(t, cwd)
	require.NotNil(t, cp.Git)
	require.Equal(t, "main", cp.Git.Branch)
	require.False(t, cp.Git.Dirty, "the checkpoint must not be counted as a dirty file")
	require.Zero(t, cp.Git.Changes)

	// A real change is still reported.
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "tracked.txt"), []byte("changed"), 0o644))
	require.NoError(t, w.Write(t.Context(), Checkpoint{}))
	require.Equal(t, 1, readCheckpoint(t, cwd).Git.Changes)
}

func TestWriteDoesNotDirtyGitStatus(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	cwd := t.TempDir()
	initGitRepo(t, gitPath, cwd)

	w := Writer{CWD: cwd}
	require.NoError(t, w.Write(t.Context(), Checkpoint{}))
	cp := readCheckpoint(t, cwd)
	require.NotNil(t, cp.Git)
	require.Equal(t, "main", cp.Git.Branch)
	require.False(t, cp.Git.Dirty)
	require.Zero(t, cp.Git.Changes)

	// A real user change is still reported.
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "tracked.txt"), []byte("changed"), 0o644))
	require.NoError(t, w.Write(t.Context(), Checkpoint{}))
	cp = readCheckpoint(t, cwd)
	require.True(t, cp.Git.Dirty)
	require.Equal(t, 1, cp.Git.Changes)
}
