//go:build windows

package megafloydweb

import (
	"os/exec"
	"syscall"
)

// setDetached starts the child without a console and in its own process
// group so it survives the parent.
func setDetached(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = syscall.CREATE_NEW_PROCESS_GROUP | syscall.DETACHED_PROCESS
}
