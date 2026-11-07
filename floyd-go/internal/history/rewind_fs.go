package history

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// A rewindDir pins a verified directory for the entire transaction. File
// operations accept single names, so an ancestor rename or symlink replacement
// cannot redirect either installation or rollback to a different directory.
type rewindDir struct{ *os.File }

func rewindName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("invalid rewind entry name: %q", name)
	}
	return nil
}

func (d *rewindDir) snapshot(name string) (fileSnapshot, os.FileMode, error) {
	file, err := d.read(name)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, 0o600, nil
	}
	if err != nil {
		return fileSnapshot{}, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fileSnapshot{}, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxTurnFileBytes {
		return fileSnapshot{}, 0, fmt.Errorf("rewind requires a bounded regular file: %s", name)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxTurnFileBytes+1))
	if err != nil {
		return fileSnapshot{}, 0, err
	}
	if len(content) > maxTurnFileBytes {
		return fileSnapshot{}, 0, fmt.Errorf("file grew beyond rewind bound: %s", name)
	}
	digest := sha256.Sum256(content)
	state := fileSnapshot{Exists: true, Hash: hex.EncodeToString(digest[:]), Binary: !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00')}
	if !state.Binary {
		state.Content = string(content)
	}
	return state, info.Mode().Perm(), nil
}

func (d *rewindDir) write(name string, data []byte, mode os.FileMode) error {
	file, err := d.create(name, mode)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func (d *rewindDir) descend(rel string) (*rewindDir, error) {
	current, err := d.child(".", false)
	if err != nil {
		return nil, err
	}
	if rel == "." {
		return current, nil
	}
	for _, name := range strings.Split(rel, string(filepath.Separator)) {
		next, err := current.child(name, false)
		current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func (d *rewindDir) temporary() (*rewindDir, string, error) {
	for range 10 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := "operation-" + hex.EncodeToString(random[:])
		if err := d.mkdir(name); errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return nil, "", err
		}
		child, err := d.child(name, false)
		return child, name, err
	}
	return nil, "", errors.New("cannot allocate a unique rewind recovery directory")
}

func sameRewindDirectory(a, b *rewindDir) error {
	left, err := a.Stat()
	if err != nil {
		return err
	}
	right, err := b.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(left, right) {
		return errors.New("workspace directory changed during rewind")
	}
	return nil
}
