//go:build darwin || linux

package approvalauthority

import (
	"errors"
	"os"
	"syscall"
)

func statUID(info os.FileInfo) (uint32, error) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("approval authority file ownership unavailable")
	}
	return s.Uid, nil
}
