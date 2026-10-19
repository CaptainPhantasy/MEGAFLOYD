//go:build windows

package megafloydweb

import (
	"os/exec"
	"syscall"
)

// Windows process creation flags. They are defined here because the
// syscall package does not export DETACHED_PROCESS on every Go release.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// setDetached starts the child without a console and in its own process
// group so it survives the parent.
func setDetached(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = createNewProcessGroup | detachedProcess
}
