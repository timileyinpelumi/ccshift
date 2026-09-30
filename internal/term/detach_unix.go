//go:build !windows

package term

import (
	"os/exec"
	"syscall"
)

// detach puts the terminal in its own session so it outlives ccshift.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
