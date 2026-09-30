package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/ui"
)

// color wraps s in an ANSI colour when writing to a terminal, unless NO_COLOR is set.
func (a *App) ui() *ui.UI { return ui.For(a.Out, func(k string) string { return a.Env[k] }) }

func (a *App) color(code, s string) string { return a.ui().Paint(ui.Style(code), s) }

func (a *App) done(msg string) { a.ui().OK("%s", msg) }

// packaged reports whether the binary belongs to a package manager, which should remove it.
func packaged(exe string) bool {
	return strings.HasPrefix(exe, "/usr/bin/") || strings.HasPrefix(exe, "/usr/local/Cellar/") || strings.HasPrefix(exe, "/opt/homebrew/")
}

func (a *App) cmdUninstall(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	keep := fs.Bool("keep-data", false, "keep saved layouts, names and settings")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 0 {
		return usageError{"usage: ccshift uninstall [--yes] [--keep-data]"}
	}
	exe := a.Executable
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	configDir := filepath.Dir(a.ConfigPath)

	fmt.Fprintln(a.Out, "This removes:")
	fmt.Fprintf(a.Out, "  the ccshift hooks and statusline from %s\n", a.settingsPath())
	if !*keep {
		fmt.Fprintf(a.Out, "  saved layouts, names and briefs in %s\n", a.Store.Dir)
		fmt.Fprintf(a.Out, "  settings in %s\n", configDir)
	}
	fmt.Fprintf(a.Out, "  the ccshift program at %s\n", exe)
	if !*yes {
		fmt.Fprint(a.Out, "Continue? [y/N] ")
		answer, _ := bufio.NewReader(a.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			return fmt.Errorf("nothing was removed")
		}
	}
	fmt.Fprintln(a.Out)

	out := a.Out
	a.Out = &strings.Builder{}
	err = a.cmdInit(ctx, []string{"--remove"})
	a.Out = out
	if err != nil {
		return fmt.Errorf("removing the hooks: %w", err)
	}
	a.done("Removed the hooks and statusline")

	if !*keep {
		for _, dir := range []string{a.Store.Dir, configDir} {
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
		a.done("Removed saved data and settings")
	}

	switch {
	case packaged(exe):
		fmt.Fprintf(a.Out, "%s The program was installed by a package manager. Remove it with one of:\n", a.color("33", "›"))
		fmt.Fprintln(a.Out, "    sudo apt remove ccshift\n    sudo dnf remove ccshift")
	case goos == "windows":
		if err := removeWindowsBinary(exe); err != nil {
			return err
		}
		a.done("Removed the program")
	default:
		if err := os.Remove(exe); err != nil {
			return fmt.Errorf("removing %s: %w", exe, err)
		}
		a.done("Removed the program")
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, a.color("1", "ccshift is uninstalled.")+" Running Claude sessions keep going; restart them to drop the hooks.")
	return nil
}

// removeWindowsBinary deletes the program after it exits, since Windows cannot delete a running
// executable, and takes its folder off the user's PATH.
func removeWindowsBinary(exe string) error {
	dir := filepath.Dir(exe)
	del := exec.Command("cmd", "/c", "ping 127.0.0.1 -n 3 >nul & del /f /q \""+exe+"\" & rmdir \""+dir+"\"")
	if err := del.Start(); err != nil {
		return err
	}
	script := `& { param($d) $p = [Environment]::GetEnvironmentVariable('Path', 'User');` +
		` [Environment]::SetEnvironmentVariable('Path', (($p -split ';') | Where-Object { $_ -and $_ -ne $d }) -join ';', 'User') }`
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script, dir).Run()
}
