package cli

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

const feedbackURL = "https://www.timileyin.dev/ccshift/feedback"

func (a *App) cmdFeedback(_ context.Context, args []string) error {
	if len(args) > 0 {
		return usageError{"feedback takes no arguments"}
	}
	q := url.Values{"os": {goos}, "version": {version}}
	if ad, err := a.adapter(""); err == nil {
		q.Set("terminal", ad.Name())
	}
	if a.Telemetry != nil {
		q.Set("id", a.Telemetry.ID())
	}
	link := feedbackURL + "?" + q.Encode()
	u := a.ui()
	if err := a.OpenURL(link); err != nil {
		u.Info("Open this link to send feedback:")
	} else {
		u.OK("Opened the feedback form in your browser.")
	}
	fmt.Fprintln(a.Out, "  "+link)
	return nil
}

func openURL(link string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
