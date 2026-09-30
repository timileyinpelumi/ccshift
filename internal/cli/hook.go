package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/layout"
	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/telemetry"
)

const (
	hookBudget = 2 * time.Second
	// An exiting Claude process is still listed for a moment after its SessionEnd hook runs.
	endedGrace = 2 * time.Minute
	// An unchanged layout is still rewritten this often, so last_seen stays current.
	refreshEvery = 24 * time.Hour
)

type logWriter struct{ st *store.Store }

func (w logWriter) Write(p []byte) (int, error) {
	w.st.Log("%s", string(p))
	return len(p), nil
}

// cmdHook never returns an error: a failing hook must not disturb the Claude session that ran it.
func (a *App) cmdHook(ctx context.Context, args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			a.Store.Log("hook %v: panic: %v", args, r)
		}
		if err != nil {
			a.Store.Log("hook %v: %v", args, err)
		}
		err = nil
	}()
	ctx, cancel := context.WithTimeout(ctx, hookBudget)
	defer cancel()
	stderr := a.Err
	a.Err = logWriter{a.Store}
	defer func() { a.Err = stderr }()
	agent := ""
	if len(args) == 3 && args[1] == "--agent" {
		agent, args = args[2], args[:1]
	}
	if len(args) != 1 {
		return fmt.Errorf("expected one event name")
	}
	in := claude.ParseHookInput(readInput(a.In))
	if agent != "" && args[0] != "session-end" {
		if err := a.registerAgent(agent, in); err != nil {
			a.Store.Log("hook %s --agent %s: %v", args[0], agent, err)
		}
	}
	a.hookSession = in.SessionID
	if args[0] == "session-start" || args[0] == "stop" {
		if err := a.recordSessionEnv(in.SessionID); err != nil {
			a.Store.Log("hook %s: %v", args[0], err)
		}
	}
	switch args[0] {
	case "session-start":
		// Done on its own first, so a resumed session is not hidden if the autosave below fails.
		if err := a.clearEnded(in.SessionID); err != nil {
			a.Store.Log("hook session-start: %v", err)
		}
		used := agent
		if used == "" {
			used = "claude"
		}
		a.record(telemetry.Event{Command: "session", Agent: used})
		a.sendLater()
		return a.autosave(ctx)
	case "stop":
		if a.Now().Sub(a.Store.LastAutosave()) >= time.Duration(a.Config.AutosaveDebounceSeconds)*time.Second {
			if err := a.autosave(ctx); err != nil {
				a.Store.Log("hook stop: %v", err)
			}
		}
		if agent == "" {
			a.contextFromTranscript(in)
		}
		return nil
	case "session-end":
		return a.sessionEnded(in)
	}
	return fmt.Errorf("unknown event %q", args[0])
}

func (a *App) clearEnded(id string) error {
	if id == "" {
		return nil
	}
	unlock, err := a.Store.TryLock(hookBudget / 4)
	if err != nil {
		return err
	}
	defer unlock()
	ended := a.Store.Ended()
	if _, ok := ended[id]; !ok {
		return nil
	}
	delete(ended, id)
	return a.Store.SetEnded(ended)
}

func (a *App) autosave(ctx context.Context) error {
	// The view is taken before the lock: it calls claude and the terminal, which is too slow to hold
	// the lock across. A hook that started earlier can therefore write a slightly older view last;
	// the next autosave corrects it, and nothing is dropped either way.
	v, err := a.view(ctx, "")
	if err != nil {
		return err
	}
	if err := a.autosaveLocked(v); err != nil {
		return err
	}
	// Terminal commands run after the lock is released, so a session ending is never kept waiting.
	a.syncTitles(ctx, v)
	return nil
}

func (a *App) autosaveLocked(v *view) error {
	unlock, err := a.Store.TryLock(hookBudget / 2)
	if err != nil {
		return err
	}
	defer unlock()
	now := a.Now()

	ended := a.Store.Ended()
	pruned := false
	for id, at := range ended {
		if now.Sub(time.Unix(at, 0)) > endedGrace {
			delete(ended, id)
			pruned = true
		}
	}
	if pruned {
		a.Store.SetEnded(ended)
	}
	excluded := a.Store.Excluded()
	for id := range a.superseded() {
		excluded[id] = 0
	}

	wsNames := layout.WorkspaceNames(v.groups)
	saved, err := a.Store.Workspaces()
	if err != nil {
		return err
	}
	for _, ws := range saved {
		if _, ok := v.groups[ws]; !ok {
			wsNames = append(wsNames, ws)
		}
	}
	liveIn := map[string]string{}
	for ws, items := range v.groups {
		for _, it := range items {
			liveIn[it.Session.ID] = ws
		}
	}
	stale := time.Duration(a.Config.StaleDays) * 24 * time.Hour
	for _, ws := range wsNames {
		var items []layout.Item
		for _, it := range v.groups[ws] {
			_, gone := ended[it.Session.ID]
			_, out := excluded[it.Session.ID]
			if !gone && !out {
				items = append(items, it)
			}
		}
		prev, ok, err := a.Store.Latest(ws)
		if err != nil {
			a.Store.Log("autosave: %v", err)
			continue
		}
		// A session whose directory now belongs to another workspace is saved there, not in both.
		carried := prev
		carried.Sessions = nil
		oldest := now
		for _, e := range prev.Sessions {
			if other, live := liveIn[e.SessionID]; live && other != ws {
				continue
			}
			carried.Sessions = append(carried.Sessions, e)
			if _, live := liveIn[e.SessionID]; live && e.LastSeen != 0 && time.Unix(e.LastSeen, 0).Before(oldest) {
				oldest = time.Unix(e.LastSeen, 0)
			}
		}
		snap := layout.Merge(carried, ok, ws, items, v.adapter.Name(), now, stale)
		if !ok && len(snap.Sessions) == 0 {
			continue
		}
		if ok && layout.SameSessions(prev, snap) && now.Sub(oldest) < refreshEvery {
			continue
		}
		if err := a.Store.SaveAuto(snap, a.Config.HistoryKeep); err != nil {
			return err
		}
		ids := make([]string, len(snap.Sessions))
		for i, e := range snap.Sessions {
			ids[i] = e.SessionID
		}
		if err := a.Store.SetOrder(ws, ids); err != nil {
			return err
		}
	}
	if err := a.saveNames(v); err != nil {
		a.Store.Log("autosave: %v", err)
	}
	// Name entries are kept for sessions that are running or saved, and dropped with the rest.
	keep := map[string]bool{}
	for id := range liveIn {
		keep[id] = true
	}
	if wss, err := a.Store.Workspaces(); err == nil {
		for _, ws := range wss {
			if snap, ok, err := a.Store.Latest(ws); err == nil && ok {
				for _, e := range snap.Sessions {
					keep[e.SessionID] = true
				}
			}
		}
	}
	if err := a.updateNames(func(m map[string]names.Entry) {
		for id, e := range m {
			if !keep[id] && now.Sub(time.Unix(e.Touched, 0)) > nameGrace {
				delete(m, id)
			}
		}
	}); err != nil {
		a.Store.Log("autosave: %v", err)
	}
	recorded := map[string]map[string]string{}
	a.Store.Load(sessionEnvFile, &recorded)
	for id := range recorded {
		// Only running sessions have a tab; a resumed one records its new tab when it starts.
		// The session whose hook this is may not be listed yet, and is kept.
		if _, running := liveIn[id]; !running && id != a.hookSession {
			delete(recorded, id)
		}
	}
	a.Store.Put(sessionEnvFile, recorded)
	return a.Store.StampAutosave(now)
}

// sessionEnded only edits saved files: SessionEnd hooks share a 1.5 second budget.
func (a *App) sessionEnded(in claude.HookInput) error {
	if in.SessionID == "" {
		return nil
	}
	unlock, err := a.Store.TryLock(hookBudget / 2)
	if err != nil {
		return err
	}
	defer unlock()
	now := a.Now()
	remove := in.UserExit()
	if remove {
		ended := a.Store.Ended()
		ended[in.SessionID] = now.Unix()
		if err := a.Store.SetEnded(ended); err != nil {
			return err
		}
	}
	if excluded := a.Store.Excluded(); len(excluded) > 0 {
		if _, ok := excluded[in.SessionID]; ok {
			delete(excluded, in.SessionID)
			a.Store.SetExcluded(excluded)
		}
	}
	wss, err := a.Store.Workspaces()
	if err != nil {
		return err
	}
	for _, ws := range wss {
		snap, ok, err := a.Store.Latest(ws)
		if err != nil || !ok {
			continue
		}
		idx := -1
		for i, e := range snap.Sessions {
			if e.SessionID == in.SessionID {
				idx = i
			}
		}
		switch {
		case idx < 0:
			continue
		case remove:
			snap.Sessions = append(snap.Sessions[:idx:idx], snap.Sessions[idx+1:]...)
			for i := range snap.Sessions {
				snap.Sessions[i].Position = i + 1
			}
		default:
			snap.Sessions[idx].Exit = layout.ExitUnclean
			snap.Sessions[idx].LastSeen = now.Unix()
		}
		snap.SavedAt = now
		if err := a.Store.SaveLatest(snap); err != nil {
			return err
		}
	}
	return nil
}
