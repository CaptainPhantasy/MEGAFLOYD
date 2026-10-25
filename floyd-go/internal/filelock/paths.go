// Package filelock coordinates known native file effects within one process.
package filelock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var locks = struct {
	sync.Mutex
	next    uint64
	held    map[uint64][]string
	changed chan struct{}
}{held: map[uint64][]string{}, changed: make(chan struct{})}

// Resolve follows each symlink before consuming subsequent parent components.
// It never changes the path passed to the actual executor. Missing components
// can name a new file; other filesystem errors remain errors.
func Resolve(cwd, name string) (string, error) {
	if !filepath.IsAbs(name) {
		if !filepath.IsAbs(cwd) {
			return "", fmt.Errorf("absolute working directory required")
		}
		name = cwd + string(filepath.Separator) + name
	}
	volume := filepath.VolumeName(name)
	current := volume + string(filepath.Separator)
	parts := strings.Split(filepath.ToSlash(strings.TrimPrefix(name, volume)), "/")
	links := 0
	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
			continue
		}
		next := current + string(filepath.Separator) + part
		info, err := os.Lstat(next)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", err
			}
			current = next
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 40 {
				return "", fmt.Errorf("too many symlinks resolving write target")
			}
			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}
			if filepath.IsAbs(target) {
				volume = filepath.VolumeName(target)
				current = volume + string(filepath.Separator)
				target = strings.TrimPrefix(target, volume)
			}
			parts = append(strings.Split(filepath.ToSlash(target), "/"), parts...)
			continue
		}
		if len(parts) > 0 && !info.IsDir() {
			return "", fmt.Errorf("write target traverses a non-directory: %s", next)
		}
		current = next
	}
	return filepath.Clean(current), nil
}

func overlaps(a, b string) (bool, error) {
	separator := string(filepath.Separator)
	if a == b || strings.HasPrefix(a, strings.TrimRight(b, separator)+separator) || strings.HasPrefix(b, strings.TrimRight(a, separator)+separator) {
		return true, nil
	}
	if same, err := sameFile(a, b); same || err != nil {
		return same, err
	}
	// New files with the same basename can have aliased existing parents.
	// Do not invent case folding for names whose filesystem identity is unknown.
	if filepath.Base(a) == filepath.Base(b) {
		return sameFile(filepath.Dir(a), filepath.Dir(b))
	}
	return false, nil
}

func sameFile(a, b string) (bool, error) {
	left, leftErr := os.Stat(a)
	if leftErr != nil && !os.IsNotExist(leftErr) {
		return false, leftErr
	}
	right, rightErr := os.Stat(b)
	if rightErr != nil && !os.IsNotExist(rightErr) {
		return false, rightErr
	}
	return leftErr == nil && rightErr == nil && os.SameFile(left, right), nil
}

// Acquire holds the named paths and their descendants until release. Reads
// and unknown effects do not receive an invented workspace-wide lock.
func Acquire(ctx context.Context, paths []string) (func(), error) {
	paths = slices.Clone(paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) == 0 {
		return func() {}, ctx.Err()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		locks.Lock()
		conflict := false
		for _, held := range locks.held {
			for _, a := range paths {
				for _, b := range held {
					overlap, err := overlaps(a, b)
					if err != nil {
						locks.Unlock()
						return nil, err
					}
					if overlap {
						conflict = true
					}
				}
			}
		}
		if !conflict {
			locks.next++
			id := locks.next
			locks.held[id] = paths
			locks.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					locks.Lock()
					delete(locks.held, id)
					close(locks.changed)
					locks.changed = make(chan struct{})
					locks.Unlock()
				})
			}, nil
		}
		changed := locks.changed
		locks.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
