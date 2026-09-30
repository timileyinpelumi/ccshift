package term

import (
	"context"
	"strings"
)

// windowsTerminal drives Windows Terminal through wt.exe. It can open tabs but cannot say what
// order they are in. Written from the wt command line documentation; not run on Windows.
type windowsTerminal struct{ x Exec }

func (w *windowsTerminal) Name() string { return "windows-terminal" }
func (w *windowsTerminal) Tier() Tier   { return LaunchOnly }

func (w *windowsTerminal) Detect(env map[string]string, ancestors []string) bool {
	if env["WT_SESSION"] != "" {
		return true
	}
	for _, c := range ancestors {
		if strings.HasPrefix(strings.ToLower(c), "windowsterminal") {
			return true
		}
	}
	return false
}

func (w *windowsTerminal) TabOf(map[string]string) (TabID, bool)         { return "", false }
func (w *windowsTerminal) List(context.Context) ([]Tab, error)           { return nil, ErrUnsupported }
func (w *windowsTerminal) SetTitle(context.Context, TabID, string) error { return ErrUnsupported }
func (w *windowsTerminal) Focus(context.Context, TabID) error            { return ErrUnsupported }

// newTab is one "new-tab" command. wt separates commands with ";", so one inside an argument is escaped.
func newTab(l Launch) []string {
	args := []string{"new-tab", "-d", l.CWD, "--title", l.Title, "--suppressApplicationTitle"}
	for _, a := range l.Argv {
		args = append(args, strings.ReplaceAll(a, ";", `\;`))
	}
	return args
}

// OpenWindow opens one new window with every session as a tab, in a single wt command.
func (w *windowsTerminal) OpenWindow(_ context.Context, _ string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	args := []string{"-w", "new"}
	for i, l := range ls {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, newTab(l)...)
	}
	return w.x.Spawn("wt.exe", args...)
}

// OpenTab adds a tab to the window wt was last used in. wt cannot place it next to a given tab.
func (w *windowsTerminal) OpenTab(_ context.Context, _ TabID, l Launch) error {
	return w.x.Spawn("wt.exe", append([]string{"-w", "0"}, newTab(l)...)...)
}
