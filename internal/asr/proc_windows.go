//go:build windows

package asr

import (
	"os/exec"
	"syscall"
)

func hideWindowProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
