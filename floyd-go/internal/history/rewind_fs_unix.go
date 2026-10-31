//go:build darwin || linux

package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openRewindRoot(path string) (*rewindDir, error) {
	// Resolve pre-existing aliases in the containing path (for example macOS
	// /tmp), then open each component without following a later substitution.
	// The workspace itself must never be a symlink.
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	current := &rewindDir{os.NewFile(uintptr(fd), "/")}
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Join(parent, filepath.Base(path)), "/"), "/") {
		next, err := current.child(part, false)
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func (d *rewindDir) child(name string, create bool) (*rewindDir, error) {
	// "." duplicates an already pinned directory without any pathname lookup.
	if name != "." {
		if err := rewindName(name); err != nil {
			return nil, err
		}
	}
	if create {
		if err := d.mkdir(name); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	fd, err := unix.Openat(int(d.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open rewind directory %s without symlinks: %w", name, err)
	}
	return &rewindDir{os.NewFile(uintptr(fd), name)}, nil
}

func (d *rewindDir) mkdir(name string) error {
	if err := rewindName(name); err != nil {
		return err
	}
	return unix.Mkdirat(int(d.Fd()), name, 0o700)
}

func (d *rewindDir) read(name string) (*os.File, error) {
	if err := rewindName(name); err != nil {
		return nil, err
	}
	// Non-blocking prevents a substituted FIFO from hanging before fstat can
	// reject it. O_NOFOLLOW rejects symlinks in the final component as well.
	fd, err := unix.Openat(int(d.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("read rewind file %s without symlinks: %w", name, err)
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (d *rewindDir) create(name string, mode os.FileMode) (*os.File, error) {
	if err := rewindName(name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(d.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (d *rewindDir) move(name string, to *rewindDir, target string) error {
	if err := errors.Join(rewindName(name), rewindName(target)); err != nil {
		return err
	}
	return renameRewindExclusive(int(d.Fd()), name, int(to.Fd()), target)
}

func (d *rewindDir) link(name string, to *rewindDir, target string) error {
	if err := errors.Join(rewindName(name), rewindName(target)); err != nil {
		return err
	}
	// Linkat never replaces an existing destination, and flags=0 does not
	// follow a source symlink if another writer substitutes the source entry.
	return unix.Linkat(int(d.Fd()), name, int(to.Fd()), target, 0)
}
