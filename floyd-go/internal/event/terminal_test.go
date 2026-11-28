package event

import "testing"

// TestTerminalUIReportsTheSetState: the flag is off unless the terminal
// interface is reported as running, and a reset returns it to off.
func TestTerminalUIReportsTheSetState(t *testing.T) {
	t.Cleanup(func() { SetTerminalUI(false) })
	SetTerminalUI(false)
	if TerminalUI() {
		t.Fatal("the terminal interface is reported as owning the terminal before it was set")
	}
	SetTerminalUI(true)
	if !TerminalUI() {
		t.Fatal("the terminal interface is not reported as owning the terminal after it was set")
	}
	SetTerminalUI(false)
	if TerminalUI() {
		t.Fatal("the terminal interface is still reported as owning the terminal after it was reset")
	}
}
