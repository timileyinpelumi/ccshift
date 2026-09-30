package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestSaveWritesSnapshotPerWorkspace(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Config.Workspaces = map[string][]string{"work": {"/w"}}
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = []liveSession{
		{id: "s-web", cwd: "/w/web", name: "web", pane: "p2", pid: 12},
		{id: "s-api", cwd: "/w/api", name: "api", pane: "p1", pid: 11},
		{id: "s-home", cwd: "/home/u", name: "home", pid: 13},
	}
	h.must(t, "save")
	snap, ok, _ := h.app.Store.Latest("work")
	if !ok || len(snap.Sessions) != 2 || snap.Sessions[0].SessionID != "s-api" || snap.Terminal != "fake" {
		t.Fatalf("work snapshot = %+v", snap)
	}
	if _, ok, _ := h.app.Store.Latest("default"); !ok {
		t.Fatal("default workspace not saved")
	}
}

func TestSaveWithNothingRunningKeepsPreviousSave(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{{id: "s1", cwd: "/p", name: "one", pid: 5}}
	h.must(t, "save")
	h.sessions = nil
	out := h.must(t, "save")
	if !strings.Contains(out, "default: no sessions running, kept the previous save (1 sessions)") {
		t.Fatalf("out = %q", out)
	}
	out = h.must(t, "save", "default")
	if !strings.Contains(out, "kept the previous save") {
		t.Fatalf("out = %q", out)
	}
	snap, _, _ := h.app.Store.Latest("default")
	if len(snap.Sessions) != 1 {
		t.Fatalf("previous save was replaced: %+v", snap)
	}
	h.must(t, "save", "default", "--force")
	snap, _, _ = h.app.Store.Latest("default")
	if len(snap.Sessions) != 0 {
		t.Fatalf("--force should save an empty layout: %+v", snap)
	}
}

func TestSaveEditReordersAndDrops(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{
		{id: "s1", cwd: "/p/1", name: "one", pid: 1, minute: 0},
		{id: "s2", cwd: "/p/2", name: "two", pid: 2, minute: 1},
		{id: "s3", cwd: "/p/3", name: "three", pid: 3, minute: 2},
	}
	h.app.Editor = func(path string) error {
		return os.WriteFile(path, []byte("# comment\ns3  three  /p/3\n\ns1  one  /p/1\n"), 0o644)
	}
	h.must(t, "save", "--edit")
	snap, _, _ := h.app.Store.Latest("default")
	if len(snap.Sessions) != 2 || snap.Sessions[0].SessionID != "s3" || snap.Sessions[1].Position != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	order, _ := h.app.Store.Order()
	if strings.Join(order["default"], ",") != "s3,s1" {
		t.Fatalf("order = %v", order)
	}
}

func TestSaveRejectsBadWorkspace(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	if code := h.run("save", "../x"); code != 2 {
		t.Fatalf("exit %d, stderr %s", code, h.err)
	}
}
