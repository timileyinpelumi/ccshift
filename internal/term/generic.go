package term

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type generic struct{ x Exec }

func (g *generic) Name() string                                  { return "generic" }
func (g *generic) Tier() Tier                                    { return WindowsOnly }
func (g *generic) Detect(map[string]string, []string) bool       { return true }
func (g *generic) TabOf(map[string]string) (TabID, bool)         { return "", false }
func (g *generic) List(context.Context) ([]Tab, error)           { return nil, ErrUnsupported }
func (g *generic) SetTitle(context.Context, TabID, string) error { return ErrUnsupported }
func (g *generic) Focus(context.Context, TabID) error            { return ErrUnsupported }

func (g *generic) OpenWindow(_ context.Context, title string, ls []Launch) error {
	term := ""
	if g.x.Env["DISPLAY"] != "" || g.x.Env["WAYLAND_DISPLAY"] != "" {
		term = g.x.Env["TERMINAL"]
		if term == "" {
			if _, err := g.x.LookPath("x-terminal-emulator"); err == nil {
				term = "x-terminal-emulator"
			}
		}
	}
	if term == "" {
		fmt.Fprintf(g.x.Out, "Can't open a terminal window for %s from here. Run these yourself:\n", title)
		PrintCommands(g.x.Out, ls)
		return ErrPrinted
	}
	for _, l := range ls {
		script := "cd " + ShellJoin([]string{l.CWD}) + " && exec " + ShellJoin(l.Argv)
		argv := append(strings.Fields(term), "-e", "sh", "-c", script)
		if err := g.x.Spawn(argv[0], argv[1:]...); err != nil {
			return fmt.Errorf("%s: %w", argv[0], err)
		}
	}
	return nil
}

// PrintCommands prints the commands for running by hand. The env -u prefix ccshift launches with
// is left off: a shell the user types into is not inside a Claude session.
func PrintCommands(w io.Writer, ls []Launch) {
	for _, l := range ls {
		fmt.Fprintf(w, "  cd %s && %s\n", ShellJoin([]string{l.CWD}), ShellJoin(withoutEnv(l.Argv)))
	}
}

func withoutEnv(argv []string) []string {
	if len(argv) > 2 && argv[1] == "exec" && argv[2] == "--" {
		return argv[3:]
	}
	if len(argv) == 0 || argv[0] != "env" {
		return argv
	}
	i := 1
	for i+1 < len(argv) && argv[i] == "-u" {
		i += 2
	}
	return argv[i:]
}
