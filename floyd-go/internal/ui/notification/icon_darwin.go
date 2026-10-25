//go:build darwin

package notification

import _ "embed"

// Icon is the MegaFloyd PNG used by OSC 99 and native notifications.
// The macOS AppleScript fallback does not support a custom icon.
//
//go:embed floyd-icon.png
var Icon []byte
