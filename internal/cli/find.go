package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func (a *App) cmdFind(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	limit := fs.Int("limit", 15, "how many sessions to show")
	here := fs.Bool("here", false, "only sessions started in this directory or below it")
	resume := fs.Int("resume", 0, "resume the session with this number from the list")
	words, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(words) == 0 {
		return usageError{"usage: ccshift find <words> [--here] [--limit n] [--resume n]"}
	}
	matches, err := claude.Search(a.Claude.ProjectsDir, words, 0)
	if err != nil {
		return err
	}
	if *here {
		kept := matches[:0]
		for _, m := range matches {
			if m.CWD == a.Cwd || strings.HasPrefix(m.CWD, a.Cwd+string(filepath.Separator)) {
				kept = append(kept, m)
			}
		}
		matches = kept
	}
	if len(matches) > *limit {
		matches = matches[:*limit]
	}
	if len(matches) == 0 {
		fmt.Fprintf(a.Out, "No past session mentions %q.\n", strings.Join(words, " "))
		return nil
	}
	home, _ := os.UserHomeDir()
	for i, m := range matches {
		name := m.Title
		if name == "" {
			name = shortID(m.SessionID)
		}
		dir := m.CWD
		if home != "" && strings.HasPrefix(dir, home) {
			dir = "~" + dir[len(home):]
		}
		fmt.Fprintf(a.Out, "%2d  %-5s %s  %s\n", i+1, age(a.Now().Sub(m.Modified)), a.color("1", name), a.color("2", dir))
		fmt.Fprintf(a.Out, "      %s\n", m.Snippet)
	}
	pick := *resume
	if pick == 0 && a.Interactive != nil && a.Interactive() {
		fmt.Fprint(a.Out, "\nResume which? (number, or Enter to stop) ")
		line, _ := bufio.NewReader(a.In).ReadString('\n')
		if n, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			pick = n
		}
	}
	if pick == 0 {
		return nil
	}
	if pick < 1 || pick > len(matches) {
		return usageError{fmt.Sprintf("there is no number %d in the list", pick)}
	}
	m := matches[pick-1]
	return a.resumeSession(ctx, m.SessionID, m.CWD, "")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// resumeSession switches to a session's tab when it is running, and otherwise resumes it here.
func (a *App) resumeSession(ctx context.Context, id, cwd, agent string) error {
	v, err := a.view(ctx, "")
	if err != nil {
		return err
	}
	for _, it := range v.order {
		if it.Session.ID != id {
			continue
		}
		if it.Matched && v.adapter.Tier() == term.Exact {
			if err := v.adapter.Focus(ctx, it.Tab); err == nil {
				fmt.Fprintf(a.Out, "%s is already running; switched to its tab.\n", it.Session.Name)
				return nil
			}
		}
		fmt.Fprintf(a.Out, "%s is already running in another tab.\n", it.Session.Name)
		return nil
	}
	if cwd != "" {
		if !a.DirExists(cwd) {
			return fmt.Errorf("the session's directory %s no longer exists", cwd)
		}
		if err := a.Chdir(cwd); err != nil {
			return err
		}
	}
	return a.Exec(layout.ResumeArgv(agent, a.ClaudeBin, id))
}
