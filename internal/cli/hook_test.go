package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func (h *harness) hook(t *testing.T, event, stdin string) {
	t.Helper()
	h.app.In = strings.NewReader(stdin)
	if code := h.run("hook", event); code != 0 {
		t.Fatalf("hook %s exited %d: %s", event, code, h.err)
	}
}

func (h *harness) at(d time.Duration) { h.app.Now = func() time.Time { return testNow.Add(d) } }

func latestIDs(t *testing.T, h *harness, ws string) []string {
	t.Helper()
	snap, _, err := h.app.Store.Latest(ws)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range snap.Sessions {
		out = append(out, e.SessionID)
	}
	return out
}

func twoSessions() []liveSession {
	return []liveSession{
		{id: "s1", cwd: "/p/1", name: "one", pane: "p1", pid: 1, minute: 0},
		{id: "s2", cwd: "/p/2", name: "two", pane: "p2", pid: 2, minute: 1},
	}
}

func TestHookShutdownKeepsEverySession(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}, {ID: "p2"}}
	h.sessions = twoSessions()
	h.hook(t, "session-start", `{"session_id":"s2","source":"startup"}`)
	if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s1,s2" {
		t.Fatalf("saved = %s", got)
	}
	// Shutdown: every process gets a signal, so each reports "other".
	h.sessions = nil
	h.hook(t, "session-end", `{"session_id":"s1","reason":"other"}`)
	h.hook(t, "session-end", `{"session_id":"s2","reason":"other"}`)
	h.at(time.Hour)
	h.hook(t, "stop", `{"session_id":"x"}`)
	snap, _, _ := h.app.Store.Latest("default")
	if len(snap.Sessions) != 2 || snap.Sessions[0].Exit != "unclean" || snap.Sessions[1].Exit != "unclean" {
		t.Fatalf("snapshot after shutdown = %+v", snap.Sessions)
	}
}

func TestHookUserExitRemovesOnlyThatSession(t *testing.T) {
	for _, reason := range []string{"prompt_input_exit", "clear", "resume"} {
		h := testApp(t, term.Exact)
		h.sessions = twoSessions()
		h.hook(t, "session-start", `{}`)
		h.hook(t, "session-end", `{"session_id":"s1","reason":"`+reason+`"}`)
		if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s2" {
			t.Fatalf("%s: saved = %s", reason, got)
		}
		// The exiting process is still listed for a moment. Another session's hook must not re-add it.
		h.at(time.Minute)
		h.hook(t, "stop", `{"session_id":"s2"}`)
		if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s2" {
			t.Fatalf("%s: exited session came back: %s", reason, got)
		}
		// Resuming it later brings it back.
		h.at(2 * time.Minute)
		h.hook(t, "session-start", `{"session_id":"s1","source":"resume"}`)
		if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s1,s2" {
			t.Fatalf("%s: resumed session missing: %s", reason, got)
		}
	}
}

func TestHookStopIsDebounced(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()[:1]
	h.hook(t, "stop", `{}`)
	h.sessions = twoSessions()
	h.at(10 * time.Second)
	h.hook(t, "stop", `{}`)
	if got := latestIDs(t, h, "default"); len(got) != 1 {
		t.Fatalf("saved inside the debounce window: %v", got)
	}
	h.at(40 * time.Second)
	h.hook(t, "stop", `{}`)
	if got := latestIDs(t, h, "default"); len(got) != 2 {
		t.Fatalf("not saved after the debounce window: %v", got)
	}
}

func TestHookWritesNothingWhenUnchanged(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	h.hook(t, "session-start", `{}`)
	h.at(time.Hour)
	h.hook(t, "session-start", `{}`)
	if hist, _ := h.app.Store.History("default"); len(hist) != 1 {
		t.Fatalf("history entries = %d", len(hist))
	}
}

func TestHookNeverFails(t *testing.T) {
	h := testApp(t, term.Exact)
	for _, c := range [][2]string{{"stop", "not json"}, {"session-end", ""}, {"nope", "{}"}, {"session-start", `{"session_id": 5}`}} {
		h.hook(t, c[0], c[1])
	}
	h.app.In = strings.NewReader("{}")
	if code := h.run("hook"); code != 0 {
		t.Fatalf("hook with no event exited %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(h.app.Store.Dir, "ccshift.log"))
	if !strings.Contains(string(b), "nope") {
		t.Fatalf("unknown event not logged: %q", b)
	}
}

func TestHookGivesUpWhenTheStoreIsLocked(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	unlock, _ := h.app.Store.Lock()
	defer unlock()
	start := time.Now()
	h.hook(t, "session-start", `{}`)
	if time.Since(start) > 3*time.Second {
		t.Fatal("hook waited too long for the lock")
	}
}
