package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/ui"
)

func (a *App) cmdSave(ctx context.Context, args []string) error {
	u := a.ui()
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	edit := fs.Bool("edit", false, "edit the order in $EDITOR before saving")
	force := fs.Bool("force", false, "save even when no sessions are running")
	termName := fs.String("terminal", "", "terminal adapter to use instead of detecting it")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) > 1 {
		return usageError{"usage: ccshift save [workspace] [--edit] [--force]"}
	}
	if len(pos) == 1 && !config.ValidWorkspaceName(pos[0]) {
		return usageError{fmt.Sprintf("invalid workspace name %q", pos[0])}
	}
	v, err := a.view(ctx, *termName)
	if err != nil {
		return err
	}
	// A session that has been handed off stays out of saves while it is still open.
	gone := a.superseded()
	for ws, items := range v.groups {
		kept := items[:0:0]
		for _, it := range items {
			if _, handedOff := gone[it.Session.ID]; !handedOff {
				kept = append(kept, it)
			}
		}
		v.groups[ws] = kept
	}
	targets := layout.WorkspaceNames(v.groups)
	if len(pos) == 1 {
		targets = pos
	} else {
		saved, err := a.Store.Workspaces()
		if err != nil {
			return err
		}
		for _, ws := range saved {
			if _, live := v.groups[ws]; !live {
				targets = append(targets, ws)
			}
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(a.Out, "No Claude sessions running, nothing saved.")
		return nil
	}
	// The editor runs before the lock is taken so an open editor never blocks other ccshift calls.
	edited := map[string][]layout.Item{}
	emptied := map[string]bool{}
	for _, ws := range targets {
		edited[ws] = v.groups[ws]
		if *edit && len(edited[ws]) > 0 {
			if edited[ws], err = a.editOrder(ws, edited[ws]); err != nil {
				return err
			}
			emptied[ws] = len(edited[ws]) == 0
		}
	}
	live := map[string]bool{}
	for _, s := range v.live {
		live[s.ID] = true
	}
	unlock, err := a.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	// Running sessions removed in the editor stay out of autosave. A plain save puts them back.
	excluded := a.Store.Excluded()
	for _, ws := range targets {
		kept := map[string]bool{}
		for _, it := range edited[ws] {
			kept[it.Session.ID] = true
		}
		for _, it := range v.groups[ws] {
			if *edit && !emptied[ws] && !kept[it.Session.ID] {
				excluded[it.Session.ID] = a.Now().Unix()
			} else {
				delete(excluded, it.Session.ID)
			}
		}
	}
	if err := a.Store.SetExcluded(excluded); err != nil {
		return err
	}
	if err := a.saveNames(v); err != nil {
		return err
	}
	for _, ws := range targets {
		items := edited[ws]
		if emptied[ws] && !*force {
			fmt.Fprintf(a.Out, "%s: every line was removed, nothing saved. Use --force to save an empty layout.\n", ws)
			continue
		}
		if len(items) == 0 && !*force {
			prev, ok, err := a.Store.Latest(ws)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(a.Out, "%s: no sessions running, nothing saved.\n", ws)
				continue
			}
			fmt.Fprintf(a.Out, "%s: no sessions running, kept the previous save (%d sessions). Drop them with ccshift forget <name>, or ccshift save %s --force.\n", ws, len(prev.Sessions), ws)
			continue
		}
		var dropped []string
		if prev, ok, err := a.Store.Latest(ws); err == nil && ok {
			kept := map[string]bool{}
			for _, it := range items {
				kept[it.Session.ID] = true
			}
			for _, e := range prev.Sessions {
				if !kept[e.SessionID] && !live[e.SessionID] {
					dropped = append(dropped, layout.Title(e))
				}
			}
		}
		snap := layout.Snapshot(ws, items, v.adapter.Name(), a.Now())
		if err := a.Store.SaveSnapshot(snap, a.Config.HistoryKeep); err != nil {
			return err
		}
		ids := make([]string, len(items))
		for i, it := range items {
			ids[i] = it.Session.ID
		}
		if err := a.Store.SetOrder(ws, ids); err != nil {
			return err
		}
		a.tally += len(items)
		u.OK("%s: saved %d sessions", u.Paint(ui.Bold, ws), len(items))
		for _, e := range snap.Sessions {
			a.listItem(u, e.Position, layout.Title(e), e.CWD)
		}
		if len(dropped) > 0 {
			fmt.Fprintf(a.Out, "  dropped %d sessions that are no longer running: %s. Undo with: ccshift restore %s --pick\n", len(dropped), strings.Join(dropped, ", "), ws)
		}
	}
	return nil
}

func (a *App) editOrder(ws string, items []layout.Item) ([]layout.Item, error) {
	f, err := os.CreateTemp("", "ccshift-order-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	fmt.Fprintf(f, "# Sessions in workspace %s, top to bottom. Reorder or delete lines, then save and quit.\n", ws)
	byID := map[string]layout.Item{}
	for _, it := range items {
		byID[it.Session.ID] = it
		fmt.Fprintf(f, "%s  %s  %s\n", it.Session.ID, displayName(it.Session), it.Session.CWD)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := a.Editor(f.Name()); err != nil {
		return nil, fmt.Errorf("editor: %w", err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	var out []layout.Item
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id := strings.Fields(line)[0]
		if it, ok := byID[id]; ok && !seen[id] {
			out = append(out, it)
			seen[id] = true
		}
	}
	return out, nil
}
