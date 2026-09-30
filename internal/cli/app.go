package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/proc"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
	"github.com/timileyinpelumi/ccshift/internal/update"
)

// version is set at release time with -ldflags.
var version = "dev"

type App struct {
	Out io.Writer
	Err io.Writer
	In  io.Reader

	Claude       *claude.Reader
	Store        *store.Store
	Config       config.Config
	Terms        []term.Adapter
	Env          map[string]string
	SelfPID      int
	EnvOf        func(pid int) (map[string]string, error)
	TTYOf        func(pid int) string
	hookSession  string // the session a running hook belongs to
	AncestorPIDs func(pid int) []int
	Comms        func(pids []int) []string
	Git          func(ctx context.Context, cwd string) (names.Git, error)
	AITitle      func(sessionID string) string
	Exec         func(argv []string) error
	RunClaude    func(ctx context.Context, cwd string, args []string, stdin string) (string, error)
	GitSummary   func(ctx context.Context, cwd string) string
	NewID        func() string
	Cwd          string
	DirExists    func(path string) bool
	ClaudeBin    string
	Now          func() time.Time
	Editor       func(path string) error
	ClaudeDir    string
	ConfigPath   string
	Releases     releases
	Interactive  func() bool
	Executable   string
	HasCommand   func(name string) bool
	Notify       func(title, body string)
	Shell        func(ctx context.Context, command string, stdin []byte) ([]byte, error)
}

type storeContext = store.ContextEntry

func NewApp() (*App, error) {
	scrubSessionEnv()
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return nil, err
	}
	st, err := store.Open(store.DefaultDir())
	if err != nil {
		return nil, err
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		bin = "claude"
	}
	x := term.SystemExec()
	cwd, _ := os.Getwd()
	exe, err := os.Executable()
	if err != nil {
		exe = "ccshift"
	}
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = filepath.Join(home, ".claude")
	}
	app := &App{
		Out: os.Stdout, Err: os.Stderr, In: os.Stdin,
		Claude: claude.NewReader(), Store: st, Config: cfg, Terms: term.All(x), Env: x.Env,
		SelfPID: os.Getpid(), EnvOf: proc.Environ, TTYOf: proc.TTY, AncestorPIDs: proc.Ancestors, Comms: comms,
		Git: gitInfo, Exec: execReplace, GitSummary: gitSummary, NewID: newSessionID, Cwd: cwd, DirExists: dirExists, ClaudeBin: bin, Now: time.Now, Editor: runEditor,
		Notify: notify, Shell: runShell, ClaudeDir: claudeDir, ConfigPath: config.DefaultPath(), Interactive: stdoutIsTerminal,
		Releases: update.Releases{Base: update.DefaultBase, OS: runtime.GOOS, Arch: runtime.GOARCH}, Executable: exe,
		HasCommand: func(name string) bool { _, err := exec.LookPath(name); return err == nil },
	}
	app.RunClaude = func(ctx context.Context, cwd string, args []string, stdin string) (string, error) {
		cmd := exec.CommandContext(ctx, app.ClaudeBin, args...)
		cmd.Dir = cwd
		cmd.Stdin = strings.NewReader(stdin)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return string(out), nil
	}
	app.AITitle = func(id string) string {
		if p, ok := app.Claude.TranscriptPath(id); ok {
			return claude.LastAITitle(p)
		}
		return ""
	}
	return app, nil
}

func (a *App) commands() []command {
	return []command{
		{"new", "start a named Claude session in this tab", (*App).cmdNew},
		{"ls", "list running Claude sessions in tab order", (*App).cmdLs},
		{"rename", "give a session a name", (*App).cmdRename},
		{"handoff", "continue a session in a fresh one, from a written brief", (*App).cmdHandoff},
		{"save", "save the open sessions of each workspace", (*App).cmdSave},
		{"restore", "reopen saved sessions in the current terminal", (*App).cmdRestore},
		{"focus", "switch to a session's tab", (*App).cmdFocus},
		{"forget", "remove a session from the latest save", (*App).cmdForget},
		{"ws", "list workspaces", (*App).cmdWs},
		{"init", "add autosave hooks and the statusline to Claude Code", (*App).cmdInit},
		{"doctor", "check the setup", (*App).cmdDoctor},
		{"update", "install the latest version", (*App).cmdUpdate},
		{"uninstall", "remove ccshift, its hooks and its saved data", (*App).cmdUninstall},
		{"exec", "run a command outside any Claude session (used by restore on Windows)", (*App).cmdExec},
		{"hook", "called by Claude Code hooks", (*App).cmdHook},
		{"statusline", "called by Claude Code as the status line command", (*App).cmdStatusline},
		{"version", "print the version", func(a *App, _ context.Context, _ []string) error {
			fmt.Fprintln(a.Out, version)
			return nil
		}},
	}
}

// quiet commands never trigger an automatic update: they are run by Claude Code, change the
// binary themselves, or print output meant for another program.
var quiet = map[string]bool{"hook": true, "statusline": true, "exec": true, "update": true, "uninstall": true, "version": true}

type command struct {
	name    string
	summary string
	run     func(a *App, ctx context.Context, args []string) error
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		a.usage(a.Out)
		return 0
	}
	for _, c := range a.commands() {
		if c.name != args[0] {
			continue
		}
		err := c.run(a, ctx, args[1:])
		if err == nil {
			if !quiet[c.name] && !slices.Contains(args, "--json") {
				a.autoUpdate(ctx)
			}
			return 0
		}
		fmt.Fprintln(a.Err, "ccshift:", err)
		var ue usageError
		if errors.As(err, &ue) {
			return 2
		}
		return 1
	}
	fmt.Fprintf(a.Err, "ccshift: unknown command %q\n\n", args[0])
	a.usage(a.Err)
	return 2
}

func (a *App) usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ccshift <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, c := range a.commands() {
		fmt.Fprintf(w, "  %-11s %s\n", c.name, c.summary)
	}
}
