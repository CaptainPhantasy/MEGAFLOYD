//go:build !windows

package megafloydweb

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// fatalSignals are the Unix signals that bypass the TUI runtime's cleanup.
func fatalSignals() []os.Signal {
	return []os.Signal{syscall.SIGHUP, syscall.SIGQUIT}
}

// reraiseFatal restores the default disposition and re-sends the signal so
// the process still dies the way the operating system intends.
func reraiseFatal(sig os.Signal) {
	signal.Reset(sig)
	if s, ok := sig.(syscall.Signal); ok {
		if err := syscall.Kill(os.Getpid(), s); err == nil {
			time.Sleep(200 * time.Millisecond)
		}
	}
	os.Exit(2)
}
