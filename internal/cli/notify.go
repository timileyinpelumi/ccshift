package cli

import (
	"context"
	"os/exec"
	"slices"
	"strings"
)

// cmdNotify shows a notification with a button and waits for it. It runs detached, started by
// notify, because the wait lasts as long as the notification stays up.
//
//	ccshift notify <title> <body> -- <command to run on click>
func (a *App) cmdNotify(_ context.Context, args []string) error {
	sep := slices.Index(args, "--")
	if sep != 2 || len(args) == 3 {
		return usageError{"usage: ccshift notify <title> <body> -- <command>"}
	}
	title, body, onClick := args[0], args[1], args[3:]
	plain := notifyArgs(title, body)
	out, err := a.NotifySend(append([]string{"--action=click=Hand off"}, plain...))
	if err != nil {
		// notify-send before libnotify 0.7.10 has no buttons.
		_, err = a.NotifySend(plain)
		return err
	}
	if strings.TrimSpace(out) == "click" {
		return a.Detach(onClick)
	}
	return nil
}

func notifySend(args []string) (string, error) {
	out, err := exec.Command("notify-send", args...).Output()
	return string(out), err
}
