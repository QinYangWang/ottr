//go:build !windows

package asr

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
