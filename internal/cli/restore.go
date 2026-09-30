package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
	"github.com/timileyinpelumi/ccshift/internal/ui"
)

func (a *App) cmdRestore(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	pick := fs.Bool("pick", false, "choose an older save of the workspace")
	dryRun := fs.Bool("dry-run", false, "show what would open without opening anything")
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 1 {
		return usageError{"usage: ccshift restore [workspace] [--pick] [--dry-run]"}
	}
	if *pick && len(pos) != 1 {
		return usageError{"--pick needs a workspace name"}
	}
	workspaces := pos
	if len(pos) == 0 {
		if workspaces, err = a.Store.Workspaces(); err != nil {
			return err
		}
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(a.Out, "Nothing saved yet. Run ccshift save first.")
		return nil
	}
	ad, err := a.adapter(*termName)
	if err != nil {
		return err
	}
	live, err := a.liveSessions(ctx)
	if err != nil {
		return fmt.Errorf("reading Claude sessions: %w", err)
	}
	running := map[string]bool{}
	background := 0
	for _, s := range live {
		running[s.ID] = true
		if s.Kind == "background" && (s.Status == "failed" || s.Status == "stopped") {
			background++
		}
	}
	transcriptExists := func(id string) bool {
		_, ok := a.Claude.TranscriptPath(id)
		return ok
	}
	failed := 0
	u := a.ui()
	for _, ws := range workspaces {
		snap, ok, err := a.snapshotFor(ws, *pick)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintf(a.Out, "%s: nothing saved\n", ws)
			continue
		}
		plan := layout.PlanRestore(snap, running, transcriptExists, a.DirExists, a.ClaudeBin)
		u.Info("%s: opening %d sessions in %s", u.Paint(ui.Bold, ws), len(plan.Launches), ad.Name())
		for i, l := range plan.Launches {
			a.listItem(u, i+1, l.Title, l.CWD)
		}
		for _, s := range plan.Skipped {
			fmt.Fprintf(a.Out, "  %s\n", u.Paint(ui.Amber, fmt.Sprintf("skipped %s: %s", layout.Title(s.Entry), s.Reason)))
		}
		if *dryRun || len(plan.Launches) == 0 {
			continue
		}
		// Recorded before launching: the printed fallback commands carry the same names.
		if err := a.recordApplied(plan.Launched); err != nil {
			return err
		}
		err = ad.OpenWindow(ctx, ws, a.isolated(plan.Launches))
		if errors.Is(err, term.ErrPrinted) {
			// The terminal printed the commands itself. Nothing is running, so nothing is recorded.
			continue
		}
		if err != nil {
			a.errorf("%s: couldn't open tabs in %s: %v", ws, ad.Name(), err)
			fmt.Fprintln(a.Out, "  Run these yourself:")
			term.PrintCommands(a.Out, plan.Launches)
			failed++
			continue
		}
		if u.Color {
			u.OK("Opened %d tabs in %s", len(plan.Launches), ad.Name())
		}
		// A session saved in two workspaces must only open once.
		for _, l := range plan.Launches {
			running[l.Argv[2]] = true
		}
		ids := make([]string, len(snap.Sessions))
		for i, e := range snap.Sessions {
			ids[i] = e.SessionID
		}
		if err := a.locked(func() error { return a.Store.SetOrder(ws, ids) }); err != nil {
			return err
		}
	}
	if background > 0 {
		fmt.Fprintf(a.Out, "\n%d background sessions stopped. They aren't restored as tabs. Bring them back with: claude respawn --all\n", background)
	}
	if failed > 0 {
		return fmt.Errorf("%d workspaces failed to open", failed)
	}
	if !*dryRun {
		a.supportNote()
	}
	return nil
}

// recordApplied notes which generated names are about to be handed to Claude with --name.
// Without it they would look like names the user chose, and stop following the branch.
func (a *App) recordApplied(launched []store.Entry) error {
	var gen []store.Entry
	for _, e := range launched {
		if e.Generated && e.Name != "" {
			gen = append(gen, e)
		}
	}
	if len(gen) == 0 {
		return nil
	}
	now := a.Now().Unix()
	return a.locked(func() error {
		return a.updateNames(func(m map[string]names.Entry) {
			for _, e := range gen {
				n := m[e.SessionID]
				n.Applied, n.Touched = e.Name, now
				m[e.SessionID] = n
			}
		})
	})
}

func (a *App) snapshotFor(ws string, pick bool) (store.Snapshot, bool, error) {
	if !pick {
		return a.Store.Latest(ws)
	}
	hist, err := a.Store.History(ws)
	if err != nil || len(hist) == 0 {
		return store.Snapshot{}, false, err
	}
	for i, h := range hist {
		kind := ""
		if h.Auto {
			kind = "  (autosave)"
		}
		fmt.Fprintf(a.Out, "%d. %s  %d sessions%s\n", i+1, h.SavedAt.Local().Format("2006-01-02 15:04"), len(h.Sessions), kind)
	}
	fmt.Fprint(a.Out, "Restore which? ")
	line, _ := bufio.NewReader(a.In).ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(hist) {
		return store.Snapshot{}, false, usageError{"no such save"}
	}
	return hist[n-1], true, nil
}

// listItem prints one numbered session under a heading.
func (a *App) listItem(u *ui.UI, n int, name, cwd string) {
	if u.Color {
		if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(cwd, home) {
			cwd = "~" + cwd[len(home):]
		}
	}
	fmt.Fprintf(a.Out, "  %s %s  %s\n", u.Paint(ui.Dim, fmt.Sprintf("%d.", n)), u.Paint(ui.Bold, name), u.Paint(ui.Dim, cwd))
}

// errorf reports a problem that does not stop the command.
func (a *App) errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	eu := ui.For(a.Err, func(k string) string { return a.Env[k] })
	if eu.Color {
		fmt.Fprintln(a.Err, eu.Paint(ui.Red, "✗")+" "+msg)
		return
	}
	fmt.Fprintln(a.Err, "ccshift: "+msg)
}
