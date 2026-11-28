package event

import "sync/atomic"

// terminalUI reports whether the root terminal interface owns the process
// terminal. While it does, nothing else may write to that terminal.
var terminalUI atomic.Bool

// SetTerminalUI records whether the terminal interface owns the process
// terminal.
func SetTerminalUI(active bool) { terminalUI.Store(active) }

// TerminalUI reports whether the terminal interface owns the process terminal.
func TerminalUI() bool { return terminalUI.Load() }
