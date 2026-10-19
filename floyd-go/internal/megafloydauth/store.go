package megafloydauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store loads and saves token bundles by file stem (see StorageName).
type Store interface {
	Load(name string) (Token, bool, error)
	Save(name string, token Token) error
	Remove(name string) error
	List() ([]string, error)
}

// FileStore persists tokens as <Dir>/<name>.json with mode 0600 inside a
// 0700 directory, matching the donor's FileTokenStorage. Writes are atomic:
// temp file, fsync, rename.
type FileStore struct {
	Dir string
}

// Load returns the stored token. A missing, unreadable, or corrupt file is
// reported as (zero token, false, nil), mirroring the donor's "treat as no
// token" behavior.
func (s FileStore) Load(name string) (Token, bool, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return Token{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Token{}, false, nil
	}
	if err != nil {
		return Token{}, false, fmt.Errorf("read token: %w", err)
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, false, nil
	}
	return token, true, nil
}

// Save writes the token atomically.
func (s FileStore) Save(name string, token Token) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	_ = os.Chmod(s.Dir, 0o700)

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.Dir, ".token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set token permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write token: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace token: %w", err)
	}
	return nil
}

// Remove deletes the token file. A missing file is not an error.
func (s FileStore) Remove(name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove token: %w", err)
	}
	return nil
}

// List returns every token file stem in the directory.
func (s FileStore) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || entry.IsDir() {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// Path returns the on-disk file for name. It is useful for status output;
// no caller should read the file through this path.
func (s FileStore) Path(name string) (string, error) {
	return s.pathFor(name)
}

func (s FileStore) pathFor(name string) (string, error) {
	if s.Dir == "" {
		return "", errors.New("megafloydauth: credentials directory is not configured")
	}
	if name == "" || strings.HasPrefix(name, ".") ||
		strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("megafloydauth: invalid token name %q", name)
	}
	return filepath.Join(s.Dir, name+".json"), nil
}
