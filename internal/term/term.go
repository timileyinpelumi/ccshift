package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type TabID string

type Tier int

const (
	Exact Tier = iota
	LaunchOnly
	WindowsOnly
)

func (t Tier) String() string {
	switch t {
	case Exact:
		return "exact"
	case LaunchOnly:
		return "launch-only"
	default:
		return "windows-only"
	}
}

type Tab struct {
	ID     TabID
	Window string
	Label  string // what ls shows for the tab, e.g. "work:2"
}

type Launch struct {
	CWD   string
	Title string
	Argv  []string
}

// goos is the operating system, replaceable in tests.
var goos = runtime.GOOS

var ErrUnsupported = errors.New("not supported by this terminal")

// ErrPrinted is returned when an adapter could not open anything and printed the commands instead.
// Nothing is running yet, so callers must not record the sessions as opened.
var ErrPrinted = errors.New("the commands were printed; nothing was opened")

type Adapter interface {
	Name() string
	Tier() Tier
	Detect(env map[string]string, ancestors []string) bool
	TabOf(env map[string]string) (TabID, bool)
	List(ctx context.Context) ([]Tab, error)
	OpenWindow(ctx context.Context, title string, ls []Launch) error
	SetTitle(ctx context.Context, id TabID, title string) error
	Focus(ctx context.Context, id TabID) error
}

// Checker is implemented by adapters that need something set up before they work.
// Check returns an error that tells the user what to change.
type Checker interface {
	Check(ctx context.Context) error
}

// TabOpener is implemented by terminals that can open a tab next to an existing one.
type TabOpener interface {
	OpenTab(ctx context.Context, near TabID, l Launch) error
}

// TabCloser is implemented by terminals that can close a tab from outside.
type TabCloser interface {
	CloseTab(ctx context.Context, id TabID) error
}

type Exec struct {
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	Spawn    func(name string, args ...string) error
	LookPath func(string) (string, error)
	Sleep    func(time.Duration)
	Env      map[string]string
	Out      io.Writer
}

func SystemExec() Exec {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return Exec{Run: run, Spawn: spawn, LookPath: exec.LookPath, Sleep: time.Sleep, Env: env, Out: os.Stdout}
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// spawn starts a terminal in its own session so it outlives ccshift.
func spawn(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	// A terminal that can't start exits within moments; one that works keeps running or exits 0.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s exited right away: %w", name, err)
		}
		return nil
	case <-time.After(400 * time.Millisecond):
		return nil
	}
}

// EnvKeys are the environment variables terminals set that say which terminal, window and tab a
// process is in. ccshift's hooks record them per session, because not every operating system
// lets one process read another's environment. TTYKey is added by ccshift itself.
var EnvKeys = []string{
	"TMUX", "TMUX_PANE",
	"KITTY_WINDOW_ID", "KITTY_PID", "KITTY_LISTEN_ON",
	"WEZTERM_PANE", "WEZTERM_UNIX_SOCKET",
	"ZELLIJ", "ZELLIJ_PANE_ID", "ZELLIJ_SESSION_NAME",
	"KONSOLE_DBUS_SERVICE", "KONSOLE_DBUS_SESSION", "KONSOLE_DBUS_WINDOW", "KONSOLE_VERSION",
	"GNOME_TERMINAL_SCREEN", "ALACRITTY_WINDOW_ID",
	"TERM_PROGRAM", "ITERM_SESSION_ID", "TERM_SESSION_ID", "GHOSTTY_RESOURCES_DIR",
	"WT_SESSION", "WT_PROFILE_ID",
}

// TTYKey holds the session's terminal device, such as /dev/ttys003, where there is one.
const TTYKey = "CCSHIFT_TTY"

// sameInstance reports whether a session's env points at the terminal instance ccshift runs in.
// Pane and window ids are only unique inside one instance.
func sameInstance(own, other map[string]string, key string) bool {
	return own[key] == "" || own[key] == other[key]
}

func All(x Exec) []Adapter {
	as := []Adapter{&zellij{x: x}, &tmux{x: x}, &kitty{x: x}, &wezterm{x: x}, &konsole{x: x},
		&iterm{x: x}, &appleTerminal{x: x}, &windowsTerminal{x: x}}
	as = append(as, launchOnlyAdapters(x)...)
	return append(as, &generic{x: x})
}

func Detect(as []Adapter, env map[string]string, ancestors []string) Adapter {
	for _, a := range as {
		if a.Detect(env, ancestors) {
			return a
		}
	}
	return as[len(as)-1]
}

func ByName(as []Adapter, name string) Adapter {
	for _, a := range as {
		if a.Name() == name {
			return a
		}
	}
	return nil
}
