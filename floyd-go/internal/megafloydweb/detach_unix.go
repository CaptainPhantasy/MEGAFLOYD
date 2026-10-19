//go:build !windows

package megafloydweb

import (
	"os/exec"
	"syscall"
)

// setDetached starts the child in its own session so it survives the
// parent and does not share the controlling terminal.
func setDetached(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
