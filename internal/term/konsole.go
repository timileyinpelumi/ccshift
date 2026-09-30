package term

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// konsole talks to Konsole over D-Bus with qdbus. Checked on Konsole 23.08: a window's sessionList
// follows the on-screen tab order, including after a tab is moved, and each tab's processes carry
// KONSOLE_DBUS_SERVICE and KONSOLE_DBUS_SESSION.
type konsole struct{ x Exec }

func (k *konsole) Name() string { return "konsole" }
func (k *konsole) Tier() Tier   { return Exact }

func (k *konsole) Detect(env map[string]string, ancestors []string) bool {
	if env["KONSOLE_DBUS_SERVICE"] != "" || env["KONSOLE_VERSION"] != "" {
		return true
	}
	for _, c := range ancestors {
		if strings.HasPrefix(c, "konsole") {
			return true
		}
	}
	return false
}

func (k *konsole) TabOf(env map[string]string) (TabID, bool) {
	id := strings.TrimPrefix(env["KONSOLE_DBUS_SESSION"], "/Sessions/")
	return TabID(id), id != "" && sameInstance(k.x.Env, env, "KONSOLE_DBUS_SERVICE")
}

func (k *konsole) tool() (string, error) {
	for _, name := range []string{"qdbus6", "qdbus", "qdbus-qt5"} {
		if _, err := k.x.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", errors.New("qdbus was not found; install qdbus (qt6-tools or qttools5) so ccshift can read Konsole's tabs")
}

// call runs one D-Bus call on the Konsole process ccshift is running in.
func (k *konsole) call(ctx context.Context, args ...string) (string, error) {
	tool, err := k.tool()
	if err != nil {
		return "", err
	}
	service := k.x.Env["KONSOLE_DBUS_SERVICE"]
	if service == "" {
		return "", errors.New("not running inside Konsole, so its tabs can't be read")
	}
	out, err := k.x.Run(ctx, tool, append([]string{service}, args...)...)
	return strings.TrimSpace(string(out)), err
}

func (k *konsole) Check(ctx context.Context) error {
	if _, err := k.tool(); err != nil {
		return err
	}
	return nil
}

var konsoleWindow = regexp.MustCompile(`^/Windows/(\d+)$`)

func (k *konsole) windows(ctx context.Context) ([]int, error) {
	out, err := k.call(ctx)
	if err != nil {
		return nil, err
	}
	var wins []int
	for _, line := range strings.Split(out, "\n") {
		if m := konsoleWindow.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			n, _ := strconv.Atoi(m[1])
			wins = append(wins, n)
		}
	}
	sort.Ints(wins)
	return wins, nil
}

func (k *konsole) List(ctx context.Context) ([]Tab, error) {
	wins, err := k.windows(ctx)
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	for wi, w := range wins {
		out, err := k.call(ctx, "/Windows/"+strconv.Itoa(w), "sessionList")
		if err != nil {
			return nil, err
		}
		for si, id := range strings.Fields(out) {
			tabs = append(tabs, Tab{ID: TabID(id), Window: strconv.Itoa(w), Label: fmt.Sprintf("%d.%d", wi+1, si+1)})
		}
	}
	return tabs, nil
}

func (k *konsole) windowOf(ctx context.Context, id TabID) (string, error) {
	tabs, err := k.List(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range tabs {
		if t.ID == id {
			return t.Window, nil
		}
	}
	return "", fmt.Errorf("konsole: tab %s not found", id)
}

// OpenWindow opens every session as a tab of one new window. konsole --new-tab only joins an
// existing window when Konsole runs as a single process, which is off by default, so the tabs come
// from a file. "-e true" stops Konsole adding a tab with a plain shell as well.
func (k *konsole) OpenWindow(_ context.Context, title string, ls []Launch) error {
	if len(ls) == 0 {
		return nil
	}
	var b strings.Builder
	for _, l := range ls {
		fmt.Fprintf(&b, "title: %s;; workdir: %s;; command: %s\n", strings.ReplaceAll(l.Title, ";;", ";"), l.CWD, ShellJoin(l.Argv))
	}
	path, err := cacheFile("konsole-"+title+".tabs", b.String())
	if err != nil {
		return err
	}
	return k.x.Spawn("konsole", "--tabs-from-file", path, "-e", "true")
}

// SetTitle sets the tab title format, where % starts a placeholder.
func (k *konsole) SetTitle(ctx context.Context, id TabID, title string) error {
	format := strings.ReplaceAll(title, "%", "%%")
	for _, tabContext := range []string{"0", "1"} {
		if _, err := k.call(ctx, "/Sessions/"+string(id), "setTabTitleFormat", tabContext, format); err != nil {
			return err
		}
	}
	return nil
}

func (k *konsole) Focus(ctx context.Context, id TabID) error {
	win, err := k.windowOf(ctx, id)
	if err != nil {
		return err
	}
	_, err = k.call(ctx, "/Windows/"+win, "setCurrentSession", string(id))
	return err
}

// OpenTab starts a shell in a new tab and has it replace itself with the command.
// Konsole's D-Bus interface has no call that starts a tab running a given command.
func (k *konsole) OpenTab(ctx context.Context, near TabID, l Launch) error {
	win, err := k.windowOf(ctx, near)
	if err != nil {
		return err
	}
	id, err := k.call(ctx, "/Windows/"+win, "newSession", "", l.CWD)
	if err != nil {
		return err
	}
	if _, err := k.call(ctx, "/Sessions/"+id, "runCommand", "exec "+ShellJoin(l.Argv)); err != nil {
		return err
	}
	k.SetTitle(ctx, TabID(id), l.Title)
	return nil
}
