//go:build windows

package execution

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Windows has no negative-PID process-group signal. The system taskkill tool
// stops the command and its descendants rather than only the shell process.
func configureProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		output, err := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").CombinedOutput()
		if err != nil {
			return fmt.Errorf("stop command process tree: %w: %s", err, output)
		}
		return nil
	}
}
