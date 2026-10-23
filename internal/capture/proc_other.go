//go:build !windows

package capture

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
