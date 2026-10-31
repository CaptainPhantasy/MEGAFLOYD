package history

import "golang.org/x/sys/unix"

func renameRewindExclusive(from int, name string, to int, target string) error {
	return unix.RenameatxNp(from, name, to, target, unix.RENAME_EXCL)
}
