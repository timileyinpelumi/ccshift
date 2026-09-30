package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func saveSnap(t *testing.T, h *harness, ws string, ids ...string) {
	t.Helper()
	s := store.Snapshot{Workspace: ws, SavedAt: testNow, Terminal: "fake"}
	for i, id := range ids {
		s.Sessions = append(s.Sessions, store.Entry{SessionID: id, CWD: "/p/" + id, Name: "name " + id, Position: i + 1})
		h.addTranscript(t, id)
	}
	if err := h.app.Store.SaveSnapshot(s, 20); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreOpensInOrderAndSkipsRunning(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1", "s2", "s3")
	h.sessions = []liveSession{{id: "s2", cwd: "/p/s2", name: "name s2", pid: 7}}
	out := h.must(t, "restore")
	if len(h.term.opened) != 1 || len(h.term.opened[0]) != 2 {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	got := h.term.opened[0]
	if got[0].Argv[2] != "s1" || got[1].Argv[2] != "s3" || got[0].CWD != "/p/s1" {
		t.Fatalf("launches = %+v", got)
	}
	if !strings.Contains(out, "skipped name s2: already running") {
		t.Fatalf("out = %s", out)
	}
}

func TestRestoreDryRunOpensNothing(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	out := h.must(t, "restore", "work", "--dry-run")
	if len(h.term.opened) != 0 || !strings.Contains(out, "1. name s1") {
		t.Fatalf("opened=%v out=%s", h.term.opened, out)
	}
}

func TestRestorePick(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "old")
	s := store.Snapshot{Workspace: "work", SavedAt: testNow.Add(time.Hour), Sessions: []store.Entry{{SessionID: "new", CWD: "/p/new", Position: 1}}}
	h.addTranscript(t, "new")
	h.app.Store.SaveSnapshot(s, 20)
	h.app.In = strings.NewReader("2\n")
	h.must(t, "restore", "work", "--pick")
	if len(h.term.opened) != 1 || h.term.opened[0][0].Argv[2] != "old" {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	if code := h.run("restore", "--pick"); code != 2 {
		t.Fatalf("--pick without workspace exit = %d", code)
	}
}

func TestRestoreNothingSaved(t *testing.T) {
	h := testApp(t, term.Exact)
	if out := h.must(t, "restore"); !strings.Contains(out, "Nothing saved yet") {
		t.Fatalf("out = %q", out)
	}
}
