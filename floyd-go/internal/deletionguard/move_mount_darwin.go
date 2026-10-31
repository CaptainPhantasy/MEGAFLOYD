//go:build darwin

package deletionguard

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func moveMountIdentity(file string, noFollow bool) (string, error) {
	if noFollow {
		info, err := os.Lstat(file)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			file = filepath.Dir(file)
		}
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(file, &stat); err != nil {
		return "", fmt.Errorf("move mount identity unavailable: %w", err)
	}
	return fmt.Sprintf("darwin:%d:%d:%s", stat.Fsid.Val[0], stat.Fsid.Val[1], unix.ByteSliceToString(stat.Mntonname[:])), nil
}
