package cli

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/names"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func nameEntries(h *harness) map[string]names.Entry {
	m := map[string]names.Entry{}
	h.app.Store.Load("names.json", &m)
	return m
}

func TestRenameSurvivesAHookThatReadNamesEarlier(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.aiTitles["s3"] = "First title"
	h.hook(t, "session-start", `{}`)
	// A hook reads names, then a rename lands, then the hook writes.
	h.aiTitles["s3"] = "Second title"
	stale, err := h.app.view(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	h.must(t, "rename", "3", "mine")
	unlock, _ := h.app.Store.Lock()
	h.app.saveNames(stale)
	unlock()
	if e := nameEntries(h)["s3"]; e.User != "mine" || e.Title != "Second title" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestNewSessionKeepsItsEntryUntilItIsLive(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.app.Cwd = "/w/acme"
	h.must(t, "new")
	// Another session's hook runs while Claude is still starting.
	h.at(time.Minute)
	h.hook(t, "session-start", `{}`)
	if e := nameEntries(h)["new-id"]; e.Applied == "" {
		t.Fatalf("entry pruned before the session was listed: %+v", nameEntries(h))
	}
	// An entry nobody claims is dropped later.
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	if _, ok := nameEntries(h)["new-id"]; ok {
		t.Fatal("unclaimed entry kept")
	}
}

func TestRestoreRecordsGeneratedNamesEvenWhenOpeningFails(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	for _, s := range h.sessions {
		h.addTranscript(t, s.id)
	}
	h.sessions = nil
	h.term.openErr = errors.New("no")
	h.run("restore")
	if e := nameEntries(h)["s1"]; e.Applied != "acme · OPS-664" {
		t.Fatalf("the printed commands carry a generated name that was not recorded: %+v", e)
	}
}

func TestTitleIsRememberedWhenTheTranscriptTailHasNone(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	delete(h.aiTitles, "s3")
	if got := lsNames(t, h); got[2] != "trueset · Add dark mode and a better tagline" {
		t.Fatalf("name flipped when the title went missing: %q", got[2])
	}
}

func TestGitFailureOrDetachedHeadKeepsTheName(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.hook(t, "session-start", `{}`)
	h.gitErr = map[string]error{"/w/acme": context.DeadlineExceeded}
	if got := lsNames(t, h); got[0] != "acme · OPS-664" || got[3] != "acme · OPS-664 #2" {
		t.Fatalf("names after a git timeout = %q", got)
	}
	h.gitErr = nil
	h.gits["/w/acme"] = names.Git{Repo: "acme", Detached: true, Default: "main"}
	if got := lsNames(t, h); got[0] != "acme · OPS-664" {
		t.Fatalf("names during a rebase = %q", got)
	}
}

func TestNumbersDoNotMoveWhenASessionExits(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.sessions = append(h.sessions, liveSession{id: "s5", cwd: "/w/acme", name: "acme-ff", pane: "p5", pid: 5, minute: 4})
	h.term.tabs = append(h.term.tabs, term.Tab{ID: "p5"})
	h.hook(t, "session-start", `{}`)
	if got := lsNames(t, h); got[3] != "acme · OPS-664 #2" || got[4] != "acme · OPS-664 #3" {
		t.Fatalf("names = %q", got)
	}
	h.sessions = h.sessions[1:]
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	got := lsNames(t, h)
	if got[2] != "acme · OPS-664 #2" || got[3] != "acme · OPS-664 #3" {
		t.Fatalf("numbers moved after the first session exited: %q", got)
	}
}

func TestGeneratedNameAvoidsANameTheUserChose(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = []liveSession{
		{id: "s1", cwd: "/w/api", name: "api", pid: 1, minute: 0},
		{id: "s2", cwd: "/w/api", name: "api-3f", pid: 2, minute: 1},
	}
	if got := lsNames(t, h); !reflect.DeepEqual(got, []string{"api", "api #2"}) {
		t.Fatalf("names = %q", got)
	}
}

func TestRenameEditWhitespaceAndReset(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.sessions[1].name = "PAY  2193" // two spaces, as Claude reports it
	h.must(t, "rename", "3", "mine")
	h.app.Editor = func(path string) error {
		b, _ := os.ReadFile(path)
		var out []string
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "s3 ") {
				line = "s3"
			}
			out = append(out, line)
		}
		return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	}
	out := h.must(t, "rename", "--edit")
	if !strings.Contains(out, "back to its generated name") || strings.Contains(out, "PAY") {
		t.Fatalf("out = %q", out)
	}
	if got := lsNames(t, h); got[2] != "trueset · Add dark mode and a better tagline" {
		t.Fatalf("names = %q", got)
	}
	if e, ok := nameEntries(h)["s2"]; ok && e.User != "" {
		t.Fatalf("an unchanged name with two spaces was treated as a rename: %+v", e)
	}
}

func TestRenameReset(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	h.must(t, "rename", "1", "mine")
	h.must(t, "rename", "mine", "--reset")
	if got := lsNames(t, h); got[0] != "acme · OPS-664" {
		t.Fatalf("names = %q", got)
	}
}

func TestNewWithANameThatStartsWithADash(t *testing.T) {
	h := testApp(t, term.Exact)
	h.must(t, "new", "--name=-wip", "--", "--model", "haiku")
	if got := h.execd[0]; got[3] != "--name=-wip" || got[4] != "--model" {
		t.Fatalf("exec = %q", got)
	}
}

func TestNewWritesNothingWhenClaudeIsMissing(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Env["FAKE_PANE"] = "p9"
	h.app.HasCommand = func(string) bool { return false }
	if code := h.run("new"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if len(nameEntries(h)) != 0 || h.term.titled != 0 {
		t.Fatalf("state written before the check: %v %v", nameEntries(h), h.term.titles)
	}
}

func TestTitleSyncSkipsATabSharedByTwoSessions(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1", Label: "w:0"}, {ID: "p2", Label: "w:0"}, {ID: "p3", Label: "w:1"}}
	h.sessions = []liveSession{
		{id: "s1", cwd: "/p/1", name: "one", pane: "p1", pid: 1},
		{id: "s2", cwd: "/p/2", name: "two", pane: "p2", pid: 2},
		{id: "s3", cwd: "/p/3", name: "three", pane: "p3", pid: 3},
	}
	h.hook(t, "session-start", `{}`)
	if len(h.term.titles) != 1 || h.term.titles["p3"] != "three" {
		t.Fatalf("titles = %v", h.term.titles)
	}
}

func TestTitleIsSetAgainWhenASessionMovesToAnotherTab(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = []liveSession{{id: "s1", cwd: "/p/1", name: "one", pane: "p1", pid: 1}}
	h.hook(t, "session-start", `{}`)
	h.sessions[0].pane = "p2"
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	if h.term.titles["p2"] != "one" {
		t.Fatalf("titles = %v", h.term.titles)
	}
}

func TestSessionEndIsNotBlockedByTitleSync(t *testing.T) {
	h := testApp(t, term.Exact)
	namedSessions(h)
	locked := false
	h.term.onSetTitle = func() {
		if unlock, err := h.app.Store.TryLock(50 * time.Millisecond); err == nil {
			unlock()
		} else {
			locked = true
		}
	}
	h.hook(t, "session-start", `{}`)
	if locked {
		t.Fatal("tab titles were set while the store lock was held")
	}
}
