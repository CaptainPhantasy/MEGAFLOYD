//go:build !windows

package question

import (
	"errors"
	"os"
	"path/filepath"
)

// PublishSubmission preserves the file sync followed by rename and parent sync.
func publishSubmission(source, destination string) error {
	if err := os.Rename(source, destination); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
