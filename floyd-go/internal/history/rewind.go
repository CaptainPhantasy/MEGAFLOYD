package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/legacy-ai/floyd/internal/filelock"
)

// TurnRestorer restores recorded native edits and then commits the transcript
// change. Every displaced file remains in a private recovery directory.
type TurnRestorer interface {
	RestoreTurns(context.Context, string, []string, string, func() error) ([]string, string, error)
}

type rewindFile struct {
	Path      string       `json:"path"`
	Start     fileSnapshot `json:"start"`
	End       fileSnapshot `json:"end"`
	Original  string       `json:"original"`
	Staged    string       `json:"staged"`
	moved     bool
	installed bool
}

// RestoreTurns requires message IDs in chronological order. Missing checkpoints,
// discontinuous edits, and changes made after a turn are refused before writes.
func (s *service) RestoreTurns(ctx context.Context, sid string, messages []string, root string, commit func() error) ([]string, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	byPath := map[string]*rewindFile{}
	for _, mid := range messages {
		turn, err := s.q.GetSessionTurnByMessage(ctx, db.GetSessionTurnByMessageParams{SessionID: sid, UserMessageID: mid})
		if err != nil || turn.Completed == 0 {
			return nil, "", fmt.Errorf("rewind requires a completed file checkpoint for every selected turn")
		}
		files, err := s.q.ListTurnFiles(ctx, db.ListTurnFilesParams{SessionID: sid, UserMessageID: mid})
		if err != nil {
			return nil, "", err
		}
		for _, file := range files {
			if !file.EndState.Valid {
				return nil, "", fmt.Errorf("missing ending checkpoint for %s", file.Path)
			}
			start, err := s.decodeSnapshot(ctx, file.StartState)
			if err != nil {
				return nil, "", err
			}
			end, err := s.decodeSnapshot(ctx, file.EndState.String)
			if err != nil {
				return nil, "", err
			}
			if start.Binary || start.Oversize || end.Binary || end.Oversize {
				return nil, "", fmt.Errorf("rewind cannot safely restore an incomplete or binary checkpoint: %s", file.Path)
			}
			if err := safeRewindPath(root, file.Path); err != nil {
				return nil, "", err
			}
			if previous := byPath[file.Path]; previous != nil {
				if !sameSnapshot(previous.End, start) {
					return nil, "", fmt.Errorf("file changed between selected turns: %s", file.Path)
				}
				previous.End = end
			} else {
				byPath[file.Path] = &rewindFile{Path: file.Path, Start: start, End: end}
			}
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	release, err := filelock.Acquire(ctx, paths)
	if err != nil {
		return nil, "", err
	}
	defer release()
	for _, path := range paths {
		if err := verifyRewindCurrent(root, byPath[path], false); err != nil {
			return nil, "", err
		}
	}
	if len(paths) == 0 {
		return paths, "", commit()
	}
	parent := filepath.Join(root, ".quarantine", "rewind")
	for _, dir := range []string{filepath.Join(root, ".quarantine"), parent} {
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			return nil, "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("unsafe rewind recovery directory: %s", dir)
		}
	}
	recovery, err := os.MkdirTemp(parent, "operation-")
	if err != nil {
		return nil, "", err
	}
	entries := make([]*rewindFile, 0, len(paths))
	for i, path := range paths {
		file := byPath[path]
		file.Original = filepath.Join(recovery, fmt.Sprintf("%d.original", i))
		file.Staged = filepath.Join(recovery, fmt.Sprintf("%d.restore", i))
		if file.Start.Exists {
			mode := os.FileMode(0o600)
			if info, err := os.Lstat(path); err == nil {
				mode = info.Mode().Perm()
			}
			if err := writeRewindFile(file.Staged, []byte(file.Start.Content), mode); err != nil {
				return nil, recovery, err
			}
		}
		entries = append(entries, file)
	}
	manifest, _ := json.MarshalIndent(struct {
		Session  string        `json:"session"`
		Messages []string      `json:"messages"`
		Files    []*rewindFile `json:"files"`
	}{sid, messages, entries}, "", "  ")
	if err := writeRewindFile(filepath.Join(recovery, "manifest.json"), manifest, 0o600); err != nil {
		return nil, recovery, err
	}
	rollback := func(cause error) ([]string, string, error) {
		var failures []error
		for i := len(entries) - 1; i >= 0; i-- {
			file := entries[i]
			if !file.moved && !file.installed {
				continue
			}
			if file.installed {
				if err := verifyRewindCurrent(root, file, true); err != nil {
					failures = append(failures, err)
					continue
				}
				if err := os.Rename(file.Path, filepath.Join(recovery, fmt.Sprintf("%d.rolled-back", i))); err != nil {
					failures = append(failures, err)
					continue
				}
			}
			if file.moved {
				if err := os.Link(file.Original, file.Path); err != nil {
					failures = append(failures, err)
				}
			}
		}
		if err := syncRewindDirectories(recovery, paths); err != nil {
			failures = append(failures, err)
		}
		if len(failures) > 0 {
			return nil, recovery, fmt.Errorf("%w; recovery retained at %s: %w", cause, recovery, errors.Join(failures...))
		}
		return nil, recovery, cause
	}
	for _, file := range entries {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		if err := verifyRewindCurrent(root, file, false); err != nil {
			return rollback(err)
		}
		if file.End.Exists {
			if err := os.Rename(file.Path, file.Original); err != nil {
				return rollback(err)
			}
			file.moved = true
			moved, err := readTurnSnapshot(file.Original)
			if err != nil || !sameSnapshot(moved, file.End) {
				return rollback(fmt.Errorf("file changed during rewind; original retained: %s", file.Path))
			}
		}
		if file.Start.Exists {
			// Link is exclusive: a concurrently created target is never replaced.
			if err := os.Link(file.Staged, file.Path); err != nil {
				return rollback(err)
			}
			file.installed = true
		}
	}
	if err := syncRewindDirectories(recovery, paths); err != nil {
		return rollback(err)
	}
	if err := commit(); err != nil {
		return rollback(err)
	}
	// The retained originals and manifest are the recovery route, including
	// interruption after files were restored but before transcript commit.
	return paths, recovery, nil
}

func syncRewindDirectories(recovery string, paths []string) error {
	dirs := map[string]bool{recovery: true, filepath.Dir(recovery): true}
	for _, path := range paths {
		dirs[filepath.Dir(path)] = true
	}
	for dir := range dirs {
		f, err := os.Open(dir)
		if err != nil {
			return err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func sameSnapshot(a, b fileSnapshot) bool { return a.Exists == b.Exists && a.Hash == b.Hash }

func safeRewindPath(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("rewind target is outside this workspace: %s", path)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if parts[0] == ".git" || parts[0] == ".quarantine" {
		return fmt.Errorf("rewind cannot modify repository or recovery metadata: %s", path)
	}
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("rewind refuses a symlink target: %s", current)
		}
	}
	return nil
}

func verifyRewindCurrent(root string, file *rewindFile, restored bool) error {
	if err := safeRewindPath(root, file.Path); err != nil {
		return err
	}
	actual, err := readTurnSnapshot(file.Path)
	if err != nil {
		return err
	}
	expected := file.End
	if restored {
		expected = file.Start
	}
	if !sameSnapshot(actual, expected) {
		return fmt.Errorf("file changed since its checkpoint; rewind preserved it: %s", file.Path)
	}
	return nil
}

func writeRewindFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
