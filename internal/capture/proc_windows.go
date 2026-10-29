//go:build windows

package capture

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps the temporary encoder from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
