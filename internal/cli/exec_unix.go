//go:build !windows

package cli

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// execReplace hands the process over to the command, so the tab runs Claude and not ccshift.
func execReplace(argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}

// shellCommand runs a command line in its own process group, killed as a whole at the deadline.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return cmd
}
