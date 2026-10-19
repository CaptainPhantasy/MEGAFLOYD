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

func TestWriteDoesNotDirtyGitStatus(t *testing.T) {
	t.Parallel()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	cwd := t.TempDir()
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
