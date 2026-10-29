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

type (
	reservationKey   struct{}
	observerLeaseKey struct{}
)

type observerLease struct {
	owner    uint64
	open     bool
	children map[uint64]struct{}
}

// AcquireWithContext reserves a native action's paths and carries its identity
// to synchronous permission observation. Ordinary nested acquisitions still
// conflict unless WithObserverLease explicitly lends that reservation.
func AcquireWithContext(ctx context.Context, paths []string) (context.Context, func(), error) {
	id, release, err := acquire(ctx, paths)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, reservationKey{}, id), release, nil
}

// WithObserverLease permits synchronous permission observers to use their
// waiting action's reservation. Each observer shell also owns a distinct lease,
// so sibling observers and unrelated writers remain mutually exclusive.
// finish closes the scope and drains actual shell leases before the parent can
// resume, even if the hook runner has already reported a timeout or cancellation.
func WithObserverLease(ctx context.Context) (context.Context, func()) {
	id, _ := ctx.Value(reservationKey{}).(uint64)
	locks.Lock()
	_, active := locks.held[id]
	locks.Unlock()
	if !active {
		return ctx, func() {}
	}
	scope := &observerLease{owner: id, open: true, children: map[uint64]struct{}{}}
	var once sync.Once
	finish := func() {
		once.Do(func() {
			locks.Lock()
			scope.open = false
			close(locks.changed)
			locks.changed = make(chan struct{})
			for len(scope.children) != 0 {
				changed := locks.changed
				locks.Unlock()
				<-changed
				locks.Lock()
			}
			locks.Unlock()
		})
	}
	return context.WithValue(ctx, observerLeaseKey{}, scope), finish
}

// WithoutObserverLease prevents an asynchronous observer from sharing a
// synchronous parent's reservation after the parent has resumed its action.
func WithoutObserverLease(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, reservationKey{}, uint64(0))
	return context.WithValue(ctx, observerLeaseKey{}, (*observerLease)(nil))
}

// Acquire holds the named paths and their descendants until release. Reads
// and unknown effects do not receive an invented workspace-wide lock.
func Acquire(ctx context.Context, paths []string) (func(), error) {
	_, release, err := acquire(ctx, paths)
	return release, err
}

func acquire(ctx context.Context, paths []string) (uint64, func(), error) {
	paths = slices.Clone(paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) == 0 {
		return 0, func() {}, ctx.Err()
	}
	scope, _ := ctx.Value(observerLeaseKey{}).(*observerLease)
	for {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		locks.Lock()
		if scope != nil {
			_, parentHeld := locks.held[scope.owner]
			if !scope.open || !parentHeld {
				locks.Unlock()
				return 0, nil, context.Canceled
			}
		}
		conflict := false
		for heldID, held := range locks.held {
			if scope != nil && heldID == scope.owner {
				continue
			}
			for _, a := range paths {
				for _, b := range held {
					overlap, err := overlaps(a, b)
					if err != nil {
						locks.Unlock()
						return 0, nil, err
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
			if scope != nil {
				scope.children[id] = struct{}{}
			}
			locks.Unlock()
			var once sync.Once
			return id, func() {
				once.Do(func() {
					locks.Lock()
					delete(locks.held, id)
					if scope != nil {
						delete(scope.children, id)
					}
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
			return 0, nil, ctx.Err()
		}
	}
}
