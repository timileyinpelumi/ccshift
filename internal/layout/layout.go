package layout

import (
	"sort"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

type Input struct {
	Sessions  []claude.Session
	EnvOf     func(pid int) (map[string]string, error)
	Adapter   term.Adapter
	Tabs      []term.Tab
	Config    config.Config
	PrevOrder map[string][]string
}

type Item struct {
	Session   claude.Session
	Workspace string
	Tab       term.TabID
	TabIndex  int
	TabLabel  string
	// Set by the CLI once names are resolved.
	ClaudeName string
	Generated  bool
	Branch     string
	Matched    bool
}

func Build(in Input) map[string][]Item {
	pos := make(map[term.TabID]int, len(in.Tabs))
	for i, t := range in.Tabs {
		pos[t.ID] = i
	}
	exact := in.Adapter != nil && in.Adapter.Tier() == term.Exact && in.EnvOf != nil
	groups := map[string][]Item{}
	for _, s := range in.Sessions {
		if s.Kind != "interactive" {
			continue
		}
		it := Item{Session: s, Workspace: in.Config.WorkspaceFor(s.CWD), TabIndex: -1}
		if exact {
			if env, err := in.EnvOf(s.PID); err == nil {
				if id, ok := in.Adapter.TabOf(env); ok {
					if i, listed := pos[id]; listed {
						it.Tab, it.TabIndex, it.TabLabel, it.Matched = id, i, in.Tabs[i].Label, true
					}
				}
			}
		}
		groups[it.Workspace] = append(groups[it.Workspace], it)
	}
	for ws, items := range groups {
		prev := map[string]int{}
		for i, id := range in.PrevOrder[ws] {
			prev[id] = i
		}
		sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j], prev) })
		groups[ws] = stabilize(items, prev)
	}
	return groups
}

// stabilize handles a workspace where only some sessions have a tab in the terminal that was read,
// because the rest are in another terminal. Putting the matched ones first would reorder the save
// every time a hook ran from a different terminal. Instead everything keeps its previous place, and
// the matched sessions are put, in tab order, into the places matched sessions had.
func stabilize(items []Item, prev map[string]int) []Item {
	var matched, known, newUnmatched []Item
	for _, it := range items {
		_, had := prev[it.Session.ID]
		switch {
		case it.Matched:
			matched = append(matched, it)
		case !had:
			newUnmatched = append(newUnmatched, it)
		}
		if had {
			known = append(known, it)
		}
	}
	if len(matched) == 0 || len(matched) == len(items) || len(known) == 0 {
		return items
	}
	sort.SliceStable(known, func(i, j int) bool { return prev[known[i].Session.ID] < prev[known[j].Session.ID] })
	out := make([]Item, 0, len(items))
	next := 0
	for _, it := range known {
		if it.Matched {
			out = append(out, matched[next])
			next++
		} else {
			out = append(out, it)
		}
	}
	out = append(out, matched[next:]...)
	return append(out, newUnmatched...)
}

func less(a, b Item, prev map[string]int) bool {
	if a.Matched != b.Matched {
		return a.Matched
	}
	if a.Matched {
		return a.TabIndex < b.TabIndex
	}
	pa, oka := prev[a.Session.ID]
	pb, okb := prev[b.Session.ID]
	if oka != okb {
		return oka
	}
	if oka {
		return pa < pb
	}
	if !a.Session.StartedAt.Equal(b.Session.StartedAt) {
		return a.Session.StartedAt.Before(b.Session.StartedAt)
	}
	return a.Session.ID < b.Session.ID
}

func WorkspaceNames(groups map[string][]Item) []string {
	names := make([]string, 0, len(groups))
	for ws := range groups {
		names = append(names, ws)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == config.DefaultWorkspace) != (names[j] == config.DefaultWorkspace) {
			return names[j] == config.DefaultWorkspace
		}
		return names[i] < names[j]
	})
	return names
}

func Flatten(groups map[string][]Item) []Item {
	var out []Item
	for _, ws := range WorkspaceNames(groups) {
		out = append(out, groups[ws]...)
	}
	return out
}

func Snapshot(ws string, items []Item, terminal string, now time.Time) store.Snapshot {
	s := store.Snapshot{Workspace: ws, SavedAt: now, Terminal: terminal, Sessions: []store.Entry{}}
	for i, it := range items {
		s.Sessions = append(s.Sessions, store.Entry{
			SessionID: it.Session.ID, CWD: it.Session.CWD, Name: it.Session.Name, Generated: it.Generated, Agent: it.Session.Agent, Position: i + 1, LastSeen: now.Unix(),
		})
	}
	return s
}

func Title(e store.Entry) string {
	if e.Name != "" {
		return e.Name
	}
	if len(e.SessionID) > 8 {
		return e.SessionID[:8]
	}
	return e.SessionID
}
