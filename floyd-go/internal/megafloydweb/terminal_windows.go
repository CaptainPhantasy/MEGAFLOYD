//go:build windows

package megafloydweb

import "os"

// fatalSignals is empty on Windows: the runtime has no SIGHUP/SIGQUIT
// terminal-integrity problem equivalent to Unix.
func fatalSignals() []os.Signal {
	return nil
}

func reraiseFatal(os.Signal) {
	os.Exit(2)
}
