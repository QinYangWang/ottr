//go:build !windows

package asr

import "os/exec"

func hideWindowProc(cmd *exec.Cmd) {}
