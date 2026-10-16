//go:build !darwin

package notification

import (
	_ "embed"
)

//go:embed floyd-icon-solo.png
var Icon []byte
