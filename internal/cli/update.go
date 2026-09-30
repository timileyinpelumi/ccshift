package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/update"
)

type releases interface {
	Latest(ctx context.Context) (string, error)
	Install(ctx context.Context, tag, exe string) error
}

const (
	updateFile  = "update.json"
	updateEvery = 24 * time.Hour
)

func (a *App) binaryPath() string {
	if real, err := filepath.EvalSymlinks(a.Executable); err == nil {
		return real
	}
	return a.Executable
}

// updatable says why this copy cannot update itself, or "" when it can.
func (a *App) updatable() string {
	switch {
	case version == "dev":
		return "This ccshift was built from source. Update it with: go install github.com/timileyinpelumi/ccshift/cmd/ccshift@latest"
	case packaged(a.binaryPath()):
		return "This ccshift was installed by a package manager. Update it there: sudo apt update && sudo apt install --only-upgrade ccshift"
	}
	return ""
}

func (a *App) cmdUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	check := fs.Bool("check", false, "only say whether a newer version exists")
	if _, err := parseArgs(fs, args); err != nil {
		return usageError{err.Error()}
	}
	if why := a.updatable(); why != "" {
		fmt.Fprintln(a.Out, why)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	u := a.ui()
	var tag string
	err := u.Spin("Checking for a new version", func() (err error) {
		tag, err = a.Releases.Latest(ctx)
		return err
	})
	if err != nil {
		return fmt.Errorf("checking for a new version: %w", err)
	}
	a.stampUpdateCheck()
	if !update.Newer(tag, version) {
		fmt.Fprintf(a.Out, "ccshift %s is the latest version.\n", version)
		return nil
	}
	if *check {
		fmt.Fprintf(a.Out, "ccshift %s is available (you have %s). Run: ccshift update\n", tag, version)
		return nil
	}
	if !u.Color {
		fmt.Fprintf(a.Out, "Downloading ccshift %s...\n", tag)
	}
	if err := u.Spin("Downloading ccshift "+tag, func() error { return a.Releases.Install(ctx, tag, a.binaryPath()) }); err != nil {
		return fmt.Errorf("updating: %w", err)
	}
	a.done(fmt.Sprintf("Updated ccshift from %s to %s.", version, tag))
	return nil
}

func (a *App) stampUpdateCheck() {
	a.Store.Put(updateFile, map[string]int64{"checked": a.Now().Unix()})
}

// autoUpdate checks for a new release at most once a day, after a command run by a person in a
// terminal, and installs it. Hooks, the statusline and piped output never trigger it, and any
// failure is only logged: a command must not fail because an update could not be fetched.
func (a *App) autoUpdate(ctx context.Context) {
	if !a.Config.AutoUpdate || a.Interactive == nil || !a.Interactive() || a.updatable() != "" {
		return
	}
	var last struct {
		Checked int64 `json:"checked"`
	}
	a.Store.Load(updateFile, &last)
	if last.Checked != 0 && a.Now().Sub(time.Unix(last.Checked, 0)) < updateEvery {
		return
	}
	a.stampUpdateCheck()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tag, err := a.Releases.Latest(ctx)
	if err != nil || !update.Newer(tag, version) {
		if err != nil {
			a.Store.Log("update check: %v", err)
		}
		return
	}
	if err := a.Releases.Install(ctx, tag, a.binaryPath()); err != nil {
		a.Store.Log("update to %s: %v", tag, err)
		return
	}
	fmt.Fprintf(a.Out, "\n%s\n", a.color("2", fmt.Sprintf("Updated ccshift to %s. auto_update = false turns this off.", tag)))
}

func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
