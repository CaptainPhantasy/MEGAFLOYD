package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrStale reports that the existing checkpoint is newer than the one being
// written. The existing file is left untouched.
var ErrStale = errors.New("memory: existing checkpoint is newer")

// gitRunner runs git with args in cwd and returns its standard output.
type gitRunner func(ctx context.Context, cwd string, args ...string) ([]byte, error)

// Writer records checkpoints for one working directory. The zero value is
// not usable; set CWD.
type Writer struct {
	// CWD is the project working directory. Relative paths resolve against
	// the process working directory.
	CWD string
	// Now supplies the checkpoint timestamp. Nil means time.Now.
	Now func() time.Time
	// GitTimeout bounds the git summary probe. Zero means
	// DefaultGitTimeout.
	GitTimeout time.Duration

	// git overrides the git probe in tests.
	git gitRunner
}

// Write records cp at [Path] for the writer's working directory.
//
// Write fills in the timestamp when it is zero, the absolute working
// directory, and the git summary when cp.Git is nil. A note that would push
// the document past [MaxSize] is truncated; metadata is never dropped. The
// write is atomic: a temporary file in the same directory is renamed into
// place with mode 0600.
//
// When an existing checkpoint carries a newer timestamp, Write returns
// [ErrStale] and leaves the file alone. An unreadable or malformed existing
// checkpoint is replaced.
func (w Writer) Write(ctx context.Context, cp Checkpoint) error {
	if w.CWD == "" {
		return errors.New("memory: working directory is empty")
	}
	cwd, err := filepath.Abs(w.CWD)
	if err != nil {
		cwd = filepath.Clean(w.CWD)
	}
	cp.CWD = cwd

	if cp.Timestamp.IsZero() {
		now := time.Now
		if w.Now != nil {
			now = w.Now
		}
		cp.Timestamp = now()
	}
	cp.Timestamp = cp.Timestamp.UTC()

	path := Path(cwd)
	if newer, err := existingNewer(path, cp.Timestamp); err == nil && newer {
		return ErrStale
	}

	if cp.Git == nil {
		cp.Git = w.gitSummary(ctx, cwd)
	}

	data, err := encode(cp, MaxSize)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// existingNewer reports whether the checkpoint at path carries a timestamp
// after ts.
func existingNewer(path string, ts time.Time) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var existing Checkpoint
	if err := json.Unmarshal(data, &existing); err != nil {
		return false, err
	}
	return existing.Timestamp.After(ts), nil
}

// gitSummary probes git for the branch and dirty state. It returns nil when
// git is unavailable or cwd is not a repository, so the checkpoint simply
// omits git data.
func (w Writer) gitSummary(ctx context.Context, cwd string) *GitState {
	run := w.git
	if run == nil {
		run = execGit
	}
	timeout := w.GitTimeout
	if timeout <= 0 {
		timeout = DefaultGitTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Limit the summary to the working directory and exclude the checkpoint
	// itself (and any leftover temp files), so recording project state never
	// makes the tree look dirty on its own.
	out, err := run(ctx, cwd,
		"status", "--porcelain=v1", "--branch",
		"--", ".",
		":(exclude)"+DirName+"/"+FileName,
		":(exclude)"+DirName+"/"+FileName+"-*.tmp",
	)
	if err != nil {
		return nil
	}
	return parseGitStatus(out)
}

// parseGitStatus parses the output of git status --porcelain=v1 --branch. It
// counts entries as the dirty summary and keeps no file names.
func parseGitStatus(out []byte) *GitState {
	state := &GitState{}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "## ") {
		state.Branch = parseBranch(strings.TrimPrefix(lines[0], "## "))
		lines = lines[1:]
	}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			state.Changes++
		}
	}
	state.Dirty = state.Changes > 0
	return state
}

// parseBranch extracts the branch name from the header line of git status
// --branch, which git formats as "branch...upstream [tracking info]".
func parseBranch(header string) string {
	switch {
	case strings.HasPrefix(header, "No commits yet on "):
		return strings.TrimSpace(strings.TrimPrefix(header, "No commits yet on "))
	case strings.HasPrefix(header, "Initial commit on "):
		return strings.TrimSpace(strings.TrimPrefix(header, "Initial commit on "))
	case header == "HEAD (no branch)":
		return ""
	}
	if i := strings.Index(header, "..."); i >= 0 {
		header = header[:i]
	}
	return strings.TrimSpace(header)
}

// execGit runs git without optional locks and without terminal prompts, so a
// checkpoint probe can never hang on credentials or write extra state.
func execGit(ctx context.Context, cwd string, args ...string) ([]byte, error) {
	full := append([]string{"-C", cwd}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	return cmd.Output()
}

// encode marshals cp, truncating the note until the result fits limit. It
// never drops metadata: when the document cannot fit without the note, it
// returns an error instead of writing an oversized file.
func encode(cp Checkpoint, limit int) ([]byte, error) {
	data, err := json.Marshal(cp)
	if err != nil {
		return nil, fmt.Errorf("memory: encode checkpoint: %w", err)
	}
	if len(data) <= limit {
		return data, nil
	}
	if cp.Note == "" {
		return nil, fmt.Errorf("memory: checkpoint metadata exceeds %d bytes", limit)
	}

	note := []rune(cp.Note)
	// The full note already does not fit, so search below its length for the
	// longest prefix that does.
	lo, hi := 0, len(note)-1
	var best []byte
	for lo <= hi {
		mid := lo + (hi-lo)/2
		cp.Note = string(note[:mid])
		candidate, err := json.Marshal(cp)
		if err != nil {
			return nil, fmt.Errorf("memory: encode checkpoint: %w", err)
		}
		if len(candidate) <= limit {
			best = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best == nil {
		return nil, fmt.Errorf("memory: checkpoint metadata exceeds %d bytes", limit)
	}
	return best, nil
}

// writeAtomic writes data to path through a temporary file in the same
// directory, then renames it into place so readers never observe a partial
// checkpoint.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memory: create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, FileName+"-*.tmp")
	if err != nil {
		return fmt.Errorf("memory: create temporary checkpoint: %w", err)
	}
	tmpName := tmp.Name()
	// A successful rename removes tmpName; the deferred Remove then reports
	// ENOENT, which is ignored.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory: set checkpoint permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory: write checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory: sync checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("memory: close checkpoint: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("memory: replace checkpoint: %w", err)
	}
	return nil
}
