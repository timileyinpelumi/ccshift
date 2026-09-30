package term

import (
	"context"
	"fmt"
	"strings"
)

// The macOS terminals below are driven with AppleScript through osascript. They were written
// from each application's scripting dictionary and have not been run on a Mac.

// asString writes s as an AppleScript string literal.
func asString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// shellLine is the command a new tab runs: go to the directory, then become the program.
func shellLine(l Launch) string {
	return "cd " + ShellJoin([]string{l.CWD}) + " && exec " + ShellJoin(l.Argv)
}

func osascript(ctx context.Context, x Exec, script string) (string, error) {
	out, err := x.Run(ctx, "osascript", "-e", script)
	return strings.TrimSpace(string(out)), err
}

// rows parses the tab-separated lines a listing script returns into tabs, numbering windows and
// the tabs inside them in the order they come. Lines are: window id, tab ordinal (or empty), id.
func rows(out string, idField, tabField int) []Tab {
	var tabs []Tab
	winNo := map[string]int{}
	tabNo := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) <= idField || f[idField] == "" {
			continue
		}
		win := f[0]
		if _, ok := winNo[win]; !ok {
			winNo[win] = len(winNo) + 1
		}
		n := 0
		if tabField >= 0 {
			fmt.Sscanf(f[tabField], "%d", &n)
		} else {
			tabNo[win]++
			n = tabNo[win]
		}
		tabs = append(tabs, Tab{ID: TabID(f[idField]), Window: win, Label: fmt.Sprintf("%d.%d", winNo[win], n)})
	}
	return tabs
}

type iterm struct{ x Exec }

func (i *iterm) Name() string { return "iterm2" }
func (i *iterm) Tier() Tier   { return Exact }

func (i *iterm) Detect(env map[string]string, _ []string) bool {
	return env["TERM_PROGRAM"] == "iTerm.app" || env["ITERM_SESSION_ID"] != ""
}

// ITERM_SESSION_ID looks like "w0t1p0:<unique id>".
func (i *iterm) TabOf(env map[string]string) (TabID, bool) {
	_, id, ok := strings.Cut(env["ITERM_SESSION_ID"], ":")
	return TabID(id), ok && id != ""
}

const itermEach = `repeat with w in windows
		set ti to 0
		repeat with t in tabs of w
			set ti to ti + 1
			repeat with s in sessions of t
				%s
			end repeat
		end repeat
	end repeat`

func (i *iterm) List(ctx context.Context) ([]Tab, error) {
	body := fmt.Sprintf(itermEach, `set out to out & (id of w) & tab & ti & tab & (unique id of s) & linefeed`)
	out, err := osascript(ctx, i.x, "tell application \"iTerm2\"\n\tset out to \"\"\n\t"+body+"\n\treturn out\nend tell")
	if err != nil {
		return nil, err
	}
	return rows(out, 2, 1), nil
}

// iTerm's command string is given to a shell explicitly, so quoting behaves the same as elsewhere.
func itermCommand(l Launch) string {
	return asString(ShellJoin([]string{"/bin/sh", "-c", shellLine(l)}))
}

func (i *iterm) OpenWindow(ctx context.Context, _ string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("tell application \"iTerm2\"\n")
	fmt.Fprintf(&b, "\tset w to (create window with default profile command %s)\n", itermCommand(ls[0]))
	fmt.Fprintf(&b, "\ttell current session of w to set name to %s\n", asString(ls[0].Title))
	for _, l := range ls[1:] {
		fmt.Fprintf(&b, "\ttell w\n\t\tset t to (create tab with default profile command %s)\n", itermCommand(l))
		fmt.Fprintf(&b, "\t\ttell current session of t to set name to %s\n\tend tell\n", asString(l.Title))
	}
	b.WriteString("\tactivate\nend tell")
	_, err := osascript(ctx, i.x, b.String())
	return err
}

func (i *iterm) each(ctx context.Context, id TabID, action string) error {
	body := fmt.Sprintf(itermEach, fmt.Sprintf("if unique id of s is %s then\n\t\t\t\t\t%s\n\t\t\t\tend if", asString(string(id)), action))
	_, err := osascript(ctx, i.x, "tell application \"iTerm2\"\n\t"+body+"\nend tell")
	return err
}

func (i *iterm) SetTitle(ctx context.Context, id TabID, title string) error {
	return i.each(ctx, id, "tell s to set name to "+asString(title))
}

func (i *iterm) Focus(ctx context.Context, id TabID) error {
	return i.each(ctx, id, "select w\n\t\t\t\t\tselect t\n\t\t\t\t\tselect s\n\t\t\t\t\tactivate")
}

func (i *iterm) OpenTab(ctx context.Context, near TabID, l Launch) error {
	action := fmt.Sprintf("tell w\n\t\t\t\t\t\tset nt to (create tab with default profile command %s)\n\t\t\t\t\t\ttell current session of nt to set name to %s\n\t\t\t\t\tend tell\n\t\t\t\t\treturn",
		itermCommand(l), asString(l.Title))
	return i.each(ctx, near, action)
}

func (i *iterm) CloseTab(ctx context.Context, id TabID) error {
	return i.each(ctx, id, "close s")
}

// appleTerminal is Terminal.app. Its tabs have no id a process can see, so a session is matched
// to its tab by the tty both report. AppleScript cannot add a tab to a Terminal window, so each
// restored session gets a window of its own.
type appleTerminal struct{ x Exec }

func (a *appleTerminal) Name() string { return "terminal" }
func (a *appleTerminal) Tier() Tier   { return Exact }

func (a *appleTerminal) Detect(env map[string]string, _ []string) bool {
	return env["TERM_PROGRAM"] == "Apple_Terminal"
}

func (a *appleTerminal) TabOf(env map[string]string) (TabID, bool) {
	tty := env[TTYKey]
	return TabID(tty), tty != ""
}

const terminalEach = `repeat with w in windows
		repeat with t in tabs of w
			%s
		end repeat
	end repeat`

func (a *appleTerminal) List(ctx context.Context) ([]Tab, error) {
	body := fmt.Sprintf(terminalEach, `set out to out & (id of w) & tab & (tty of t) & linefeed`)
	out, err := osascript(ctx, a.x, "tell application \"Terminal\"\n\tset out to \"\"\n\t"+body+"\n\treturn out\nend tell")
	if err != nil {
		return nil, err
	}
	return rows(out, 1, -1), nil
}

func (a *appleTerminal) OpenWindow(ctx context.Context, _ string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("tell application \"Terminal\"\n")
	for _, l := range ls {
		fmt.Fprintf(&b, "\tset t to do script %s\n\tset custom title of t to %s\n", asString(shellLine(l)), asString(l.Title))
	}
	b.WriteString("\tactivate\nend tell")
	_, err := osascript(ctx, a.x, b.String())
	return err
}

func (a *appleTerminal) each(ctx context.Context, id TabID, action string) error {
	body := fmt.Sprintf(terminalEach, fmt.Sprintf("if tty of t is %s then\n\t\t\t\t%s\n\t\t\tend if", asString(string(id)), action))
	_, err := osascript(ctx, a.x, "tell application \"Terminal\"\n\t"+body+"\nend tell")
	return err
}

func (a *appleTerminal) SetTitle(ctx context.Context, id TabID, title string) error {
	return a.each(ctx, id, "set custom title of t to "+asString(title))
}

func (a *appleTerminal) Focus(ctx context.Context, id TabID) error {
	return a.each(ctx, id, "set selected of t to true\n\t\t\t\tset index of w to 1\n\t\t\t\tactivate")
}
