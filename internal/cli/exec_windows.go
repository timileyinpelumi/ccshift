package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// execReplace runs the command and exits with its status. Windows cannot replace a process.
func execReplace(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// shellCommand uses sh when Git Bash or similar provides one, since that is what Claude Code
// runs statusline commands with, and cmd otherwise.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if sh, err := exec.LookPath("sh"); err == nil {
		return exec.CommandContext(ctx, sh, "-c", command)
	}
	return exec.CommandContext(ctx, "cmd", "/C", command)
}
