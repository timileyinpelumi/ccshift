package layout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/claude"
	"github.com/timileyinpelumi/ccshift/internal/config"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

type fakeAdapter struct{ tier term.Tier }

func (f fakeAdapter) Name() string                                            { return "fake" }
func (f fakeAdapter) Tier() term.Tier                                         { return f.tier }
func (f fakeAdapter) Detect(map[string]string, []string) bool                 { return true }
func (f fakeAdapter) List(context.Context) ([]term.Tab, error)                { return nil, nil }
func (f fakeAdapter) OpenWindow(context.Context, string, []term.Launch) error { return nil }
func (f fakeAdapter) SetTitle(context.Context, term.TabID, string) error      { return nil }
func (f fakeAdapter) Focus(context.Context, term.TabID) error                 { return nil }
func (f fakeAdapter) TabOf(env map[string]string) (term.TabID, bool) {
	p := env["PANE"]
	return term.TabID(p), p != ""
}

var t0 = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)

func sess(id string, pid int, cwd string, minute int) claude.Session {
	return claude.Session{ID: id, PID: pid, CWD: cwd, Kind: "interactive", Name: "n-" + id, StartedAt: t0.Add(time.Duration(minute) * time.Minute)}
}

func order(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Session.ID)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildExactOrdersByTab(t *testing.T) {
	panes := map[int]string{1: "%5", 2: "%1", 3: "%9"}
	in := Input{
		Sessions: []claude.Session{
			sess("a", 1, "/w/api", 0),
			sess("b", 2, "/w/web", 1),
			sess("c", 3, "/w/x", 2),
			sess("d", 4, "/w/y", 3), // no pane in env
			{ID: "bg", Kind: "background", CWD: "/w"},
		},
		EnvOf: func(pid int) (map[string]string, error) {
			if pid == 4 {
				return nil, errors.New("gone")
			}
			return map[string]string{"PANE": panes[pid]}, nil
		},
		Adapter: fakeAdapter{term.Exact},
		// %9 is not listed, so c counts as unmatched.
		Tabs:   []term.Tab{{ID: "%1"}, {ID: "%3"}, {ID: "%5"}},
		Config: config.Config{Workspaces: map[string][]string{"work": {"/w"}}},
	}
	g := Build(in)
	if len(g) != 1 {
		t.Fatalf("groups = %v", g)
	}
	if got := order(g["work"]); !eq(got, []string{"b", "a", "c", "d"}) {
		t.Fatalf("order = %v", got)
	}
	if it := g["work"][0]; !it.Matched || it.Tab != "%1" || it.TabIndex != 0 {
		t.Fatalf("first item = %+v", it)
	}
	if it := g["work"][2]; it.Matched || it.TabIndex != -1 {
		t.Fatalf("unlisted pane should be unmatched: %+v", it)
	}
}

func TestBuildLaunchOnlyUsesPreviousOrder(t *testing.T) {
	in := Input{
		Sessions:  []claude.Session{sess("a", 1, "/p/a", 0), sess("b", 2, "/p/b", 1), sess("c", 3, "/p/c", 2), sess("new", 4, "/p/n", 3)},
		Adapter:   fakeAdapter{term.LaunchOnly},
		PrevOrder: map[string][]string{config.DefaultWorkspace: {"c", "gone", "a", "b"}},
	}
	if got := order(Build(in)[config.DefaultWorkspace]); !eq(got, []string{"c", "a", "b", "new"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestWorkspaceNamesAndFlatten(t *testing.T) {
	g := map[string][]Item{
		"default":  {{Session: claude.Session{ID: "d"}}},
		"work":     {{Session: claude.Session{ID: "w1"}}, {Session: claude.Session{ID: "w2"}}},
		"personal": {{Session: claude.Session{ID: "p"}}},
	}
	if got := WorkspaceNames(g); !eq(got, []string{"personal", "work", "default"}) {
		t.Fatalf("names = %v", got)
	}
	if got := order(Flatten(g)); !eq(got, []string{"p", "w1", "w2", "d"}) {
		t.Fatalf("flatten = %v", got)
	}
}

func TestSnapshotAndTitle(t *testing.T) {
	items := []Item{{Session: sess("abcdef123456", 1, "/w/a", 0)}, {Session: claude.Session{ID: "0123456789", CWD: "/w/b"}}}
	s := Snapshot("work", items, "tmux", t0)
	if s.Workspace != "work" || s.Terminal != "tmux" || !s.SavedAt.Equal(t0) || len(s.Sessions) != 2 {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.Sessions[1].Position != 2 || s.Sessions[0].Name != "n-abcdef123456" {
		t.Fatalf("entries = %+v", s.Sessions)
	}
	if Title(s.Sessions[0]) != "n-abcdef123456" || Title(s.Sessions[1]) != "01234567" {
		t.Fatalf("titles = %q %q", Title(s.Sessions[0]), Title(s.Sessions[1]))
	}
	if Title(store.Entry{SessionID: "abc"}) != "abc" {
		t.Fatal("short id title")
	}
}

func TestBuildKeepsOrderStableWhenOnlySomeSessionsHaveTabs(t *testing.T) {
	// a and c are in the terminal the hook ran in; b and d are in another one.
	sessions := []claude.Session{sess("a", 1, "/p/a", 0), sess("b", 2, "/p/b", 1), sess("c", 3, "/p/c", 2), sess("d", 4, "/p/d", 3)}
	prev := map[string][]string{config.DefaultWorkspace: {"b", "c", "d", "a"}}
	panes := map[int]string{1: "%1", 3: "%2"}
	in := Input{
		Sessions: sessions, Adapter: fakeAdapter{term.Exact}, PrevOrder: prev,
		EnvOf: func(pid int) (map[string]string, error) { return map[string]string{"PANE": panes[pid]}, nil },
		Tabs:  []term.Tab{{ID: "%1"}, {ID: "%2"}},
	}
	// The matched sessions (a before c) take the slots matched sessions had; b and d stay put.
	if got := order(Build(in)[config.DefaultWorkspace]); !eq(got, []string{"b", "a", "d", "c"}) {
		t.Fatalf("order = %v", got)
	}
}
