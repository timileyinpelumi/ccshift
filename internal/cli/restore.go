package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
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
	live, err := a.Claude.Live(ctx)
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
		fmt.Fprintf(a.Out, "%s: opening %d sessions in %s\n", ws, len(plan.Launches), ad.Name())
		for i, l := range plan.Launches {
			fmt.Fprintf(a.Out, "  %d. %s  %s\n", i+1, l.Title, l.CWD)
		}
		for _, s := range plan.Skipped {
			fmt.Fprintf(a.Out, "  skipped %s: %s\n", layout.Title(s.Entry), s.Reason)
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
			fmt.Fprintf(a.Err, "ccshift: %s: couldn't open tabs in %s: %v\n", ws, ad.Name(), err)
			fmt.Fprintln(a.Out, "  Run these yourself:")
			term.PrintCommands(a.Out, plan.Launches)
			failed++
			continue
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
