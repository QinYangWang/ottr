package media

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps ffprobe from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
