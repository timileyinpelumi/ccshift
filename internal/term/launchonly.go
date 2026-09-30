package term

import (
	"context"
	"fmt"
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
	batch  func(ls []Launch) []string
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

func (a *launchOnly) OpenWindow(_ context.Context, _ string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	if a.batch != nil {
		argv := a.batch(ls)
		return a.x.Spawn(argv[0], argv[1:]...)
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
		&launchOnly{name: "konsole", tier: LaunchOnly, envKey: "KONSOLE_VERSION", procs: []string{"konsole"}, x: x,
			open: func(i int, l Launch) []string {
				args := []string{"konsole"}
				if i > 0 {
					args = append(args, "--new-tab")
				}
				return append(append(args, "--workdir", l.CWD, "-e"), l.Argv...)
			}},
		&launchOnly{name: "gnome-terminal", tier: LaunchOnly, envKey: "GNOME_TERMINAL_SCREEN", procs: []string{"gnome-terminal"}, x: x,
			open: func(i int, l Launch) []string {
				return append([]string{"gnome-terminal", pick(i == 0, "--window", "--tab"), "--working-directory=" + l.CWD, "--"}, l.Argv...)
			}},
		&launchOnly{name: "ptyxis", tier: LaunchOnly, procs: []string{"ptyxis"}, x: x,
			open: func(i int, l Launch) []string {
				return []string{"ptyxis", pick(i == 0, "--new-window", "--tab"), "-d", l.CWD, "-x", ShellJoin(l.Argv)}
			}},
		&launchOnly{name: "xfce4-terminal", tier: LaunchOnly, procs: []string{"xfce4-terminal"}, x: x,
			batch: func(ls []Launch) []string {
				args := []string{"xfce4-terminal"}
				for i, l := range ls {
					args = append(args, pick(i == 0, "--window", "--tab"), "--working-directory="+l.CWD, "--title="+l.Title, "--command="+ShellJoin(l.Argv))
				}
				return args
			}},
		&launchOnly{name: "tilix", tier: LaunchOnly, procs: []string{"tilix"}, x: x,
			open: func(i int, l Launch) []string {
				args := []string{"tilix"}
				if i > 0 {
					args = append(args, "--action=app-new-session")
				}
				return append(args, "--working-directory="+l.CWD, "--command="+ShellJoin(l.Argv))
			}},
		&launchOnly{name: "ghostty", tier: WindowsOnly, procs: []string{"ghostty"}, x: x,
			open: func(_ int, l Launch) []string {
				return append([]string{"ghostty", "--working-directory=" + l.CWD, "--title=" + l.Title, "-e"}, l.Argv...)
			}},
		&launchOnly{name: "alacritty", tier: WindowsOnly, envKey: "ALACRITTY_WINDOW_ID", procs: []string{"alacritty"}, x: x,
			open: func(_ int, l Launch) []string {
				return append([]string{"alacritty", "--working-directory=" + l.CWD, "--title=" + l.Title, "-e"}, l.Argv...)
			}},
		&launchOnly{name: "foot", tier: WindowsOnly, procs: []string{"foot"}, x: x,
			open: func(_ int, l Launch) []string {
				return append([]string{"foot", "--working-directory=" + l.CWD, "--title=" + l.Title}, l.Argv...)
			}},
	}
}
