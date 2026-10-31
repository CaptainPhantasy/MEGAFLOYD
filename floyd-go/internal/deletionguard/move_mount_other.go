//go:build !linux && !darwin

package deletionguard

import "errors"

func moveMountIdentity(string, bool) (string, error) {
	return "", errors.New("move mount identity is unavailable on this platform")
}
