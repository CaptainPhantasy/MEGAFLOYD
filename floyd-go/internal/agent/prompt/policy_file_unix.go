//go:build unix

package prompt

import (
	"os"

	"golang.org/x/sys/unix"
)

// openPolicyFile rejects a symlink at open time, including a link swapped in
// after directory discovery. Nonblocking open lets Stat reject a FIFO safely.
func openPolicyFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
