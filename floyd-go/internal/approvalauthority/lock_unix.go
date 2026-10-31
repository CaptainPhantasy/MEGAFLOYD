//go:build darwin || linux

package approvalauthority

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func lockStorage(dir string) (func(), error) {
	fd, err := unix.Open(filepath.Join(dir, ".authority-lock"), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("approval authority storage already in use: %w", err)
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = unix.Close(fd) }, nil
}
