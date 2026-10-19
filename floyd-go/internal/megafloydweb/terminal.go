package megafloydweb

import (
	"io"
	"os"
	"os/signal"
	"sync"
)

// TerminalResetSequence restores the terminal modes the TUI enables:
// Kitty CSI-u, modifyOtherKeys, every mouse tracking mode, and the cursor.
const TerminalResetSequence = "\x1b[<u\x1b[>4;0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?25h"

// WriteTerminalReset writes the restore sequence to w.
func WriteTerminalReset(w io.Writer) error {
	_, err := io.WriteString(w, TerminalResetSequence)
	return err
}

var fatalOnce sync.Once

// RestoreTerminalOnFatalSignals installs handlers for the fatal signals the
// TUI runtime does not cover (SIGQUIT and SIGHUP on Unix). active reports
// whether the TUI currently owns the terminal.
//
// The handler writes the reset sequence once, restores the default signal
// disposition, and re-raises, so a SIGQUIT still produces its goroutine dump
// and the exit status keeps its signal meaning. Normal exits, panics inside
// the TUI, SIGINT, and SIGTERM are already restored by the TUI runtime and
// are deliberately not handled here.
func RestoreTerminalOnFatalSignals(active func() bool) {
	fatalOnce.Do(func() {
		installFatalSignalRestore(active)
	})
}

func installFatalSignalRestore(active func() bool) {
	signals := fatalSignals()
	if len(signals) == 0 || active == nil {
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	go func() {
		sig := <-ch
		if active() {
			_ = WriteTerminalReset(os.Stdout)
		}
		signal.Stop(ch)
		reraiseFatal(sig)
	}()
}
