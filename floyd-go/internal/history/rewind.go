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
	directory *rewindDir
	name      string
	mode      os.FileMode
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
	if len(paths) == 0 {
		return paths, "", commit()
	}
	workspace, err := openRewindRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer workspace.Close()
	directories := []*rewindDir{workspace}
	for _, path := range paths {
		file := byPath[path]
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return nil, "", err
		}
		file.directory, err = workspace.descend(rel)
		if err != nil {
			return nil, "", err
		}
		defer file.directory.Close()
		directories = append(directories, file.directory)
		file.name = filepath.Base(path)
		if err := verifyRewindCurrent(file, false); err != nil {
			return nil, "", err
		}
	}
	quarantine, err := workspace.child(".quarantine", true)
	if err != nil {
		return nil, "", err
	}
	defer quarantine.Close()
	parent, err := quarantine.child("rewind", true)
	if err != nil {
		return nil, "", err
	}
	defer parent.Close()
	recoveryDir, recoveryName, err := parent.temporary()
	if err != nil {
		return nil, "", err
	}
	defer recoveryDir.Close()
	directories = append(directories, quarantine, parent, recoveryDir)
	recovery := filepath.Join(root, ".quarantine", "rewind", recoveryName)
	entries := make([]*rewindFile, 0, len(paths))
	for i, path := range paths {
		file := byPath[path]
		file.Original = filepath.Join(recovery, fmt.Sprintf("%d.original", i))
		file.Staged = filepath.Join(recovery, fmt.Sprintf("%d.restore", i))
		if file.Start.Exists {
			if err := recoveryDir.write(filepath.Base(file.Staged), []byte(file.Start.Content), file.mode); err != nil {
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
	if err := recoveryDir.write("manifest.json", manifest, 0o600); err != nil {
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
				if err := verifyRewindCurrent(file, true); err != nil {
					failures = append(failures, err)
					continue
				}
				if err := file.directory.move(file.name, recoveryDir, fmt.Sprintf("%d.rolled-back", i)); err != nil {
					failures = append(failures, err)
					continue
				}
			}
			if file.moved {
				if err := recoveryDir.link(filepath.Base(file.Original), file.directory, file.name); err != nil {
					failures = append(failures, err)
				}
			}
		}
		if err := syncRewindDirectories(directories...); err != nil {
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
		if err := verifyRewindBinding(root, workspace, file); err != nil {
			return rollback(err)
		}
		if err := verifyRewindCurrent(file, false); err != nil {
			return rollback(err)
		}
		if file.End.Exists {
			if err := file.directory.move(file.name, recoveryDir, filepath.Base(file.Original)); err != nil {
				return rollback(err)
			}
			file.moved = true
			moved, _, err := recoveryDir.snapshot(filepath.Base(file.Original))
			if err != nil || !sameSnapshot(moved, file.End) {
				return rollback(fmt.Errorf("file changed during rewind; original retained: %s", file.Path))
			}
		}
		if file.Start.Exists {
			// Link is exclusive: a concurrently created target is never replaced.
			if err := recoveryDir.link(filepath.Base(file.Staged), file.directory, file.name); err != nil {
				return rollback(err)
			}
			file.installed = true
		}
	}
	if err := syncRewindDirectories(directories...); err != nil {
		return rollback(err)
	}
	for _, file := range entries {
		if err := verifyRewindBinding(root, workspace, file); err != nil {
			return rollback(err)
		}
		if err := verifyRewindCurrent(file, true); err != nil {
			return rollback(err)
		}
	}
	if err := commit(); err != nil {
		return rollback(err)
	}
	// The retained originals and manifest are the recovery route, including
	// interruption after files were restored but before transcript commit.
	return paths, recovery, nil
}

func syncRewindDirectories(dirs ...*rewindDir) error {
	for _, dir := range dirs {
		if err := dir.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// Detect namespace changes before committing, while all actual operations stay
// bound to the original directories even if a rename happens after this check.
// Rollback deliberately uses the pinned directories without this check.
func verifyRewindBinding(root string, workspace *rewindDir, file *rewindFile) error {
	current, err := openRewindRoot(root)
	if err != nil {
		return err
	}
	defer current.Close()
	if err := sameRewindDirectory(workspace, current); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, filepath.Dir(file.Path))
	if err != nil {
		return err
	}
	parent, err := workspace.descend(rel)
	if err != nil {
		return err
	}
	defer parent.Close()
	return sameRewindDirectory(file.directory, parent)
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

	return nil
}

func verifyRewindCurrent(file *rewindFile, restored bool) error {
	actual, mode, err := file.directory.snapshot(file.name)
	if err != nil {
		return err
	}
	file.mode = mode
	expected := file.End
	if restored {
		expected = file.Start
	}
	if !sameSnapshot(actual, expected) {
		return fmt.Errorf("file changed since its checkpoint; rewind preserved it: %s", file.Path)
	}
	return nil
}
