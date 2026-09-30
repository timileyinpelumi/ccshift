package term

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type launchOnly struct {
	name   string
	tier   Tier
	envKey string
	procs  []string
	x      Exec
	open   func(i int, l Launch) []string
	batch  func(ls []Launch, title string) ([]string, error)
}

func (a *launchOnly) Name() string { return a.name }
func (a *launchOnly) Tier() Tier   { return a.tier }

func (a *launchOnly) Detect(env map[string]string, ancestors []string) bool {
	if a.envKey != "" && env[a.envKey] != "" {
		return true
	}
	for _, c := range ancestors {
		for _, p := range a.procs {
			if strings.HasPrefix(c, p) {
				return true
			}
		}
	}
	return false
}

func (a *launchOnly) TabOf(map[string]string) (TabID, bool)         { return "", false }
func (a *launchOnly) List(context.Context) ([]Tab, error)           { return nil, ErrUnsupported }
func (a *launchOnly) SetTitle(context.Context, TabID, string) error { return ErrUnsupported }
func (a *launchOnly) Focus(context.Context, TabID) error            { return ErrUnsupported }

func (a *launchOnly) OpenWindow(_ context.Context, title string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	if a.batch != nil {
		argv, err := a.batch(ls, title)
		if err != nil {
			return fmt.Errorf("%s: %w", a.name, err)
		}
		err = a.x.Spawn(argv[0], argv[1:]...)
		// An adapter with both forms uses the one-command form first and falls back to tab by tab.
		if err == nil || a.open == nil {
			return err
		}
	}
	for i, l := range ls {
		argv := a.open(i, l)
		if err := a.x.Spawn(argv[0], argv[1:]...); err != nil {
			return fmt.Errorf("%s: %w", a.name, err)
		}
		// New tabs attach to the most recently focused window, so wait for each to appear.
		if a.tier == LaunchOnly {
			if i == 0 {
				a.x.Sleep(1500 * time.Millisecond)
			} else {
				a.x.Sleep(600 * time.Millisecond)
			}
		}
	}
	return nil
}

func pick(first bool, a, b string) string {
	if first {
		return a
	}
	return b
}

func launchOnlyAdapters(x Exec) []Adapter {
	return []Adapter{
		// One command opens the window and all its tabs. Opening them one at a time relies on the new
		// window being the focused one when the next tab arrives, which does not always hold.
		// --command is deprecated in gnome-terminal, so the tab-by-tab form stays as the fallback.
		&launchOnly{name: "gnome-terminal", tier: LaunchOnly, envKey: "GNOME_TERMINAL_SCREEN", procs: []string{"gnome-terminal"}, x: x,
			batch: func(ls []Launch, _ string) ([]string, error) {
				args := []string{"gnome-terminal"}
				for i, l := range ls {
					args = append(args, pick(i == 0, "--window", "--tab"), "--working-directory="+l.CWD, "--title="+l.Title, "--command="+ShellJoin(l.Argv))
				}
				return args, nil
			},
			open: func(i int, l Launch) []string {
				return append([]string{"gnome-terminal", pick(i == 0, "--window", "--tab"), "--working-directory=" + l.CWD, "--"}, l.Argv...)
			}},
		&launchOnly{name: "ptyxis", tier: LaunchOnly, procs: []string{"ptyxis"}, x: x,
			open: func(i int, l Launch) []string {
				return []string{"ptyxis", pick(i == 0, "--new-window", "--tab"), "-d", l.CWD, "-x", ShellJoin(l.Argv)}
			}},
		&launchOnly{name: "xfce4-terminal", tier: LaunchOnly, procs: []string{"xfce4-terminal"}, x: x,
			// Options before the first --tab describe the window xfce4-terminal opens anyway.
			// Starting with --window would open a second one.
			batch: func(ls []Launch, _ string) ([]string, error) {
				args := []string{"xfce4-terminal"}
				for i, l := range ls {
					if i > 0 {
						args = append(args, "--tab")
					}
					args = append(args, "--working-directory="+l.CWD, "--title="+l.Title, "--command="+ShellJoin(l.Argv))
				}
				return args, nil
			}},
		&launchOnly{name: "tilix", tier: LaunchOnly, procs: []string{"tilix"}, x: x,
			open: func(i int, l Launch) []string {
				args := []string{"tilix"}
				if i > 0 {
					args = append(args, "--action=app-new-session")
				}
				return append(args, "--working-directory="+l.CWD, "--command="+ShellJoin(l.Argv))
			}},
		&launchOnly{name: "ghostty", tier: WindowsOnly, envKey: "GHOSTTY_RESOURCES_DIR", procs: []string{"ghostty"}, x: x,
			open: func(_ int, l Launch) []string {
				return append(macApp("Ghostty", "ghostty", "--working-directory="+l.CWD, "--title="+l.Title, "-e"), l.Argv...)
			}},
		&launchOnly{name: "alacritty", tier: WindowsOnly, envKey: "ALACRITTY_WINDOW_ID", procs: []string{"alacritty"}, x: x,
			open: func(_ int, l Launch) []string {
				return append(macApp("Alacritty", "alacritty", "--working-directory="+l.CWD, "--title="+l.Title, "-e"), l.Argv...)
			}},
		&launchOnly{name: "foot", tier: WindowsOnly, procs: []string{"foot"}, x: x,
			open: func(_ int, l Launch) []string {
				return append([]string{"foot", "--working-directory=" + l.CWD, "--title=" + l.Title}, l.Argv...)
			}},
	}
}

// cacheFile writes a file a terminal reads after ccshift has exited. One per name, overwritten each time.
func cacheFile(name, content string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "ccshift", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(content), 0o600)
}

// macApp starts a terminal that is installed as an application on macOS, where its command is
// usually not on PATH. "open -n" starts a new instance and passes on what follows --args.
func macApp(app, command string, args ...string) []string {
	if goos == "darwin" {
		return append([]string{"open", "-na", app, "--args"}, args...)
	}
	return append([]string{command}, args...)
}
