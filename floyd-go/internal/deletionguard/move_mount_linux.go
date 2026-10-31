//go:build linux

package deletionguard

import (
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sys/unix"
)

func moveMountIdentity(file string, noFollow bool) (string, error) {
	flags := 0
	if noFollow {
		flags = unix.AT_SYMLINK_NOFOLLOW
	}
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, file, flags, unix.STATX_MNT_ID, &stat); err != nil {
		return "", fmt.Errorf("move mount identity unavailable: %w", err)
	}
	if stat.Mask&unix.STATX_MNT_ID == 0 || stat.Mnt_id == 0 {
		return "", errors.New("move mount identity unavailable: STATX_MNT_ID is required")
	}
	return "linux:" + strconv.FormatUint(stat.Mnt_id, 10), nil
}
