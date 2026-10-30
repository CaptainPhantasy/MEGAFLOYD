//go:build !unix && !windows

package prompt

import (
	"errors"
	"os"
)

func openPolicyFile(string) (*os.File, error) {
	return nil, errors.New("safe policy file opening is unsupported on this platform")
}
