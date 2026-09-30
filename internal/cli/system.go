package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/proc"
)

func comms(pids []int) []string {
	out := make([]string, 0, len(pids))
	for _, p := range pids {
		if c, err := proc.Comm(p); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func git(ctx context.Context, cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", cwd}, args...)...).Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return strings.TrimSpace(string(out)), err
}

// gitInfo returns an error only when it could not find out (a timeout, a missing directory).
// A directory that is not a repository is not an error.
func gitInfo(ctx context.Context, cwd string) (names.Git, error) {
	if !dirExists(cwd) {
		return names.Git{}, os.ErrNotExist
	}
	g := names.Git{Repo: filepath.Base(cwd)}
	common, err := git(ctx, cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return g, nil
	}
	if err != nil {
		return names.Git{}, err
	}
	// The common dir belongs to the main repository, so a linked worktree gets the repository's name.
	if filepath.Base(common) == ".git" {
		g.Repo = filepath.Base(filepath.Dir(common))
	}
	branch, err := git(ctx, cwd, "symbolic-ref", "-q", "--short", "HEAD")
	if errors.As(err, &exit) {
		g.Detached = true
	} else if err != nil {
		return names.Git{}, err
	}
	g.Branch = branch
	if def, err := git(ctx, cwd, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"); err == nil {
		g.Default = strings.TrimPrefix(def, "origin/")
	} else if !errors.As(err, &exit) {
		return names.Git{}, err
	}
	return g, nil
}

// execReplace hands the process over to Claude, so the tab runs Claude and not ccshift.
func execReplace(argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func runEditor(path string) error {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	f := strings.Fields(ed)
	cmd := exec.Command(f[0], append(f[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func notify(title, body string) {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return
	}
	cmd := exec.Command(path, "--app-name=ccshift", title, body)
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

func runShell(ctx context.Context, command string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = append(os.Environ(), statuslineGuard+"=1")
	// Kill the whole process group at the deadline, and stop waiting on a pipe a grandchild still holds.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Output()
}

// readInput reads what Claude Code piped in. Run by hand from a terminal there is nothing to read.
func readInput(r io.Reader) []byte {
	if f, ok := r.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			return nil
		}
	}
	b, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return b
}

// gitSummary describes the repository for the handoff extract: branch, uncommitted files, diff size.
func gitSummary(ctx context.Context, cwd string) string {
	var b strings.Builder
	for _, c := range [][]string{{"branch", "--show-current"}, {"status", "--short"}, {"diff", "--stat"}, {"log", "--oneline", "-5"}} {
		out, err := git(ctx, cwd, c...)
		if err != nil {
			continue
		}
		if r := []rune(out); len(r) > 4000 {
			out = string(r[:4000]) + "\n[…]"
		}
		fmt.Fprintf(&b, "$ git %s\n%s\n\n", strings.Join(c, " "), out)
	}
	return strings.TrimSpace(b.String())
}
