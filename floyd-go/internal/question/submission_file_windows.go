package question

import (
	"os"

	"golang.org/x/sys/windows"
)

// PublishSubmission uses a write-through rename after the answer file is synced.
// Windows cannot FlushFileBuffers on an os.Open read-only directory handle.
func publishSubmission(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: source, New: destination, Err: err}
	}
	return nil
}
