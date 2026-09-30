package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/store"
)

type savedHit struct {
	snap store.Snapshot
	idx  int
}

func (a *App) cmdForget(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("forget", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return usageError{err.Error()}
	}
	if len(pos) != 1 {
		return usageError{"usage: ccshift forget <name|session-id-prefix>"}
	}
	spec := pos[0]
	// Asked before the lock is taken: it calls claude, and hooks wait on the lock.
	live, err := a.liveSessions(ctx)
	if err != nil {
		return fmt.Errorf("reading Claude sessions: %w", err)
	}
	unlock, err := a.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	wss, err := a.Store.Workspaces()
	if err != nil {
		return err
	}
	var exact, prefix []savedHit
	lower := strings.ToLower(spec)
	for _, ws := range wss {
		snap, ok, err := a.Store.Latest(ws)
		if err != nil {
			a.errorf("skipping workspace %s, its save can't be read (%v)", ws, err)
			continue
		}
		if !ok {
			continue
		}
		for i, e := range snap.Sessions {
			name := strings.ToLower(e.Name)
			switch {
			case name == lower || e.SessionID == spec:
				exact = append(exact, savedHit{snap, i})
			case strings.HasPrefix(name, lower) || (len(spec) >= 4 && strings.HasPrefix(e.SessionID, spec)):
				prefix = append(prefix, savedHit{snap, i})
			}
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = prefix
	}
	if len(hits) == 0 {
		return fmt.Errorf("no saved session matches %q", spec)
	}
	first := hits[0].snap.Sessions[hits[0].idx]
	for _, h := range hits[1:] {
		if h.snap.Sessions[h.idx].SessionID != first.SessionID {
			names := make([]string, len(hits))
			for i, h := range hits {
				names[i] = layout.Title(h.snap.Sessions[h.idx])
			}
			return fmt.Errorf("%q matches more than one saved session: %s", spec, strings.Join(names, ", "))
		}
	}
	for _, s := range live {
		if s.ID == first.SessionID {
			return fmt.Errorf("%s is still running; exit it first", layout.Title(first))
		}
	}
	// The same session can be saved in more than one workspace after a config change.
	var from []string
	for _, h := range hits {
		rest := append(append([]store.Entry{}, h.snap.Sessions[:h.idx]...), h.snap.Sessions[h.idx+1:]...)
		for i := range rest {
			rest[i].Position = i + 1
		}
		h.snap.Sessions = rest
		h.snap.SavedAt = a.Now()
		if err := a.Store.SaveSnapshot(h.snap, a.Config.HistoryKeep); err != nil {
			return err
		}
		from = append(from, h.snap.Workspace)
	}
	a.ui().OK("Removed %s from %s.", layout.Title(first), strings.Join(from, ", "))
	return nil
}
