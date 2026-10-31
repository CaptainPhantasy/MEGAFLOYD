//go:build !darwin && !linux

package history

import (
	"errors"
	"os"
)

var errRewindUnsupported = errors.New("safe file rewind is unsupported on this platform")

func openRewindRoot(string) (*rewindDir, error)                 { return nil, errRewindUnsupported }
func (*rewindDir) child(string, bool) (*rewindDir, error)       { return nil, errRewindUnsupported }
func (*rewindDir) mkdir(string) error                           { return errRewindUnsupported }
func (*rewindDir) read(string) (*os.File, error)                { return nil, errRewindUnsupported }
func (*rewindDir) create(string, os.FileMode) (*os.File, error) { return nil, errRewindUnsupported }
func (*rewindDir) move(string, *rewindDir, string) error        { return errRewindUnsupported }
func (*rewindDir) link(string, *rewindDir, string) error        { return errRewindUnsupported }
