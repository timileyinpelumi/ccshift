package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func namedSessions(h *harness) {
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}, {ID: "p4"}}
	h.sessions = []liveSession{
		{id: "s1", cwd: "/w/acme", name: "acme-d7", pane: "p1", pid: 1, minute: 0},
		{id: "s2", cwd: "/w/acme", name: "PAY 2193", pane: "p2", pid: 2, minute: 1},
		{id: "s3", cwd: "/w/trueset", name: "trueset-0a", pane: "p3", pid: 3, minute: 2},
		{id: "s4", cwd: "/w/acme", name: "acme-41", pane: "p4", pid: 4, minute: 3},
	}
	h.gits = map[string]names.Git{
		"/w/acme":    {Repo: "acme", Branch: "ops-664-refunds", Default: "main"},
		"/w/trueset": {Repo: "trueset", Branch: "main", Default: "main"},
	}
	h.aiTitles = map[string]string{"s3": "Add dark mode and a better tagline for the whole site"}
}

func lsNames(t *testing.T, h *harness) []string {
	t.Helper()
	var out []string
	lines := strings.Split(strings.TrimSpace(h.must(t, "ls")), "\n")
	// The header is ASCII, so its byte offsets are rune offsets into the rows.
	from, to := strings.Index(lines[0], "NAME"), strings.Index(lines[0], "WORKSPACE")
	for _, line := range lines[1:] {
		out = append(out, strings.TrimSpace(string([]rune(line)[from:to])))
	}
	return out
}

func TestLsShowsGeneratedNames(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	want := []string{"acme · OPS-664", "PAY 2193", "trueset · Add dark mode and a better tagline", "acme · OPS-664 #2"}
	if got := lsNames(t, h); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q", got)
	}
	if !strings.Contains(h.out.String(), "ops-664-refunds") {
		t.Fatalf("branch column missing:\n%s", h.out)
	}
}

func TestAutosaveStoresGeneratedNamesAndSetsTabTitles(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	snap, _, _ := h.app.Store.Latest("default")
	if snap.Sessions[0].Name != "acme · OPS-664" || !snap.Sessions[0].Generated || snap.Sessions[1].Generated {
		t.Fatalf("entries = %+v", snap.Sessions[:2])
	}
	if h.term.titles["p1"] != "acme · OPS-664" || h.term.titles["p2"] != "PAY 2193" || h.term.titled != 4 {
		t.Fatalf("titles = %v (%d calls)", h.term.titles, h.term.titled)
	}
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	if h.term.titled != 4 {
		t.Fatalf("unchanged titles were set again: %d calls", h.term.titled)
	}
	// A branch switch changes the generated name and the tab title with it.
	h.gits["/w/acme"] = names.Git{Repo: "acme", Branch: "pay-2200", Default: "main"}
	h.at(2 * time.Hour)
	h.hook(t, "session-start", `{}`)
	if h.term.titles["p1"] != "acme · PAY-2200" {
		t.Fatalf("title after branch switch = %q", h.term.titles["p1"])
	}
}

func TestTabTitleSyncCanBeTurnedOff(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.app.Config.SyncTabTitles = false
	h.hook(t, "session-start", `{}`)
	if h.term.titled != 0 {
		t.Fatalf("titles set with sync off: %v", h.term.titles)
	}
}

func TestRestoredGeneratedNameKeepsFollowingTheBranch(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	for _, s := range h.sessions {
		h.addTranscript(t, s.id)
	}
	saved := h.sessions
	h.sessions = nil
	h.must(t, "restore")
	argv := h.term.opened[0][0].Argv
	if argv[len(argv)-1] != "--name=acme · OPS-664" {
		t.Fatalf("argv = %q", argv)
	}
	// After the restore Claude reports the name ccshift gave it.
	saved[0].name = "acme · OPS-664"
	h.sessions = saved
	h.gits["/w/acme"] = names.Git{Repo: "acme", Branch: "pay-2200", Default: "main"}
	if got := lsNames(t, h); got[0] != "acme · PAY-2200" {
		t.Fatalf("a restored generated name froze: %q", got)
	}
}

func TestRename(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	out := h.must(t, "rename", "3", "site", "redesign")
	if !strings.Contains(out, "site redesign") || !strings.Contains(out, "/rename") {
		t.Fatalf("out = %q", out)
	}
	if got := lsNames(t, h); got[2] != "site redesign" {
		t.Fatalf("names = %q", got)
	}
	if h.term.titles["p3"] != "site redesign" {
		t.Fatalf("tab title = %q", h.term.titles["p3"])
	}
	snap, _, _ := h.app.Store.Latest("default")
	if snap.Sessions[2].Name != "site redesign" || snap.Sessions[2].Generated {
		t.Fatalf("saved entry = %+v", snap.Sessions[2])
	}
	// Renaming by the new name works, and the user then renames inside Claude: Claude's name wins.
	h.must(t, "rename", "site redesign", "site v2")
	h.sessions[2].name = "from claude"
	if got := lsNames(t, h); got[2] != "from claude" {
		t.Fatalf("names = %q", got)
	}
}

func TestRenameCurrentSession(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.app.AncestorPIDs = func(int) []int { return []int{77, 2} }
	h.must(t, "rename", ".", "login fix")
	if got := lsNames(t, h); got[1] != "login fix" {
		t.Fatalf("names = %q", got)
	}
}

func TestRenameRejectsBadInput(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	for _, args := range [][]string{{"rename"}, {"rename", "1"}, {"rename", "1", "  "}, {"rename", "99", "x"}, {"rename", "--edit", "1"}} {
		if code := h.run(args...); code == 0 {
			t.Errorf("%v should fail", args)
		}
	}
}

func TestRenameEdit(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	var shown string
	h.app.Editor = func(path string) error {
		b, _ := os.ReadFile(path)
		shown = string(b)
		edited := strings.Replace(shown, "acme · OPS-664 #2", "refund emails", 1)
		edited = strings.Replace(edited, "PAY 2193", "PAY 2193", 1)
		return os.WriteFile(path, []byte(edited), 0o644)
	}
	out := h.must(t, "rename", "--edit")
	if !strings.Contains(shown, "s4  acme · OPS-664 #2") {
		t.Fatalf("editor content:\n%s", shown)
	}
	if !strings.Contains(out, "Renamed acme · OPS-664 #2 to refund emails.") {
		t.Fatalf("out = %q", out)
	}
	if got := lsNames(t, h); got[3] != "refund emails" || got[0] != "acme · OPS-664" {
		t.Fatalf("names = %q", got)
	}
}

func TestNew(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.app.Cwd = "/w/acme"
	h.app.Env["FAKE_PANE"] = "p9"
	h.must(t, "new", "--", "--model", "haiku")
	want := []string{"/bin/claude", "--session-id", "new-id", "--name=acme · OPS-664 #3", "--model", "haiku"}
	if len(h.execd) != 1 || !reflect.DeepEqual(h.execd[0], want) {
		t.Fatalf("exec = %q", h.execd)
	}
	if h.term.titles["p9"] != "acme · OPS-664 #3" {
		t.Fatalf("own tab title = %q", h.term.titles["p9"])
	}
	// The name was generated, so once the session is live it keeps following the branch.
	h.sessions = append(h.sessions, liveSession{id: "new-id", cwd: "/w/acme", name: "acme · OPS-664 #3", pane: "p9", pid: 9, minute: 9})
	h.term.tabs = append(h.term.tabs, term.Tab{ID: "p9"})
	h.gits["/w/acme"] = names.Git{Repo: "acme", Branch: "main", Default: "main"}
	if got := lsNames(t, h); got[4] != "acme #3" {
		t.Fatalf("names = %q", got)
	}

	h.execd = nil
	h.must(t, "new", "my", "name")
	if got := h.execd[0]; got[3] != "--name=my name" || len(got) != 4 {
		t.Fatalf("exec = %q", got)
	}
}

func TestNameEntriesOfGoneSessionsArePruned(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	h.must(t, "rename", "1", "kept")
	h.must(t, "rename", "2", "gone soon")
	h.hook(t, "session-end", `{"session_id":"s2","reason":"prompt_input_exit"}`)
	h.sessions = append(h.sessions[:1], h.sessions[2:]...)
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	var entries map[string]names.Entry
	h.app.Store.Load("names.json", &entries)
	if _, ok := entries["s2"]; ok {
		t.Fatalf("entry for an exited session was kept: %+v", entries)
	}
	if entries["s1"].User != "kept" {
		t.Fatalf("entries = %+v", entries)
	}
}
