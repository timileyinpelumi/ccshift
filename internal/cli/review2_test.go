package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/settings"
	"github.com/timileyinpelumi/ccshift/internal/store"
	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestLongUnchangedLayoutSurvivesAShutdown(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	h.hook(t, "session-start", `{}`)
	h.at(13 * 24 * time.Hour)
	h.hook(t, "stop", `{}`)
	h.at(20 * 24 * time.Hour)
	h.hook(t, "stop", `{}`)
	h.sessions = nil
	h.hook(t, "session-end", `{"session_id":"s1","reason":"other"}`)
	h.hook(t, "session-end", `{"session_id":"s2","reason":"other"}`)
	h.at(21 * 24 * time.Hour)
	h.hook(t, "session-start", `{}`)
	if got := latestIDs(t, h, "default"); len(got) != 2 {
		t.Fatalf("sessions that ran yesterday were dropped as stale: %v", got)
	}
}

func TestAutosaveDoesNotPushOutManualSaves(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "default", "manual1")
	h.sessions = twoSessions()
	for i := range 40 {
		h.at(time.Duration(i) * time.Minute)
		h.sessions[0].name = "rename-" + string(rune('a'+i%26))
		h.hook(t, "session-start", `{}`)
	}
	hist, _ := h.app.Store.History("default")
	manual, auto := 0, 0
	for _, s := range hist {
		if s.Auto {
			auto++
		} else {
			manual++
		}
	}
	if manual != 1 || auto != 1 {
		t.Fatalf("40 autosaves in 40 minutes: manual=%d auto=%d", manual, auto)
	}
	h.at(3 * time.Hour)
	h.sessions[0].name = "later"
	h.hook(t, "session-start", `{}`)
	if hist, _ = h.app.Store.History("default"); len(hist) != 3 {
		t.Fatalf("an autosave an hour later should add one history entry, got %d", len(hist))
	}
	h.app.In = strings.NewReader("9\n")
	h.run("restore", "default", "--pick", "--dry-run")
	if !strings.Contains(h.out.String(), "autosave") {
		t.Fatalf("--pick should label autosaves:\n%s", h.out)
	}
}

func TestSessionThatMovedWorkspaceIsNotKeptInBoth(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	h.hook(t, "session-start", `{}`)
	h.app.Config.Workspaces = map[string][]string{"work": {"/p/1"}}
	h.at(time.Minute)
	h.hook(t, "session-start", `{}`)
	if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s2" {
		t.Fatalf("default = %s", got)
	}
	if got := strings.Join(latestIDs(t, h, "work"), ","); got != "s1" {
		t.Fatalf("work = %s", got)
	}
}

func TestForgetRemovesASessionSavedInTwoWorkspaces(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "a", "s1", "s2")
	saveSnap(t, h, "b", "s1")
	h.must(t, "forget", "name s1")
	if len(latestIDs(t, h, "a")) != 1 || len(latestIDs(t, h, "b")) != 0 {
		t.Fatalf("a=%v b=%v", latestIDs(t, h, "a"), latestIDs(t, h, "b"))
	}
}

func TestSessionRemovedInEditStaysOutOfAutosave(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = twoSessions()
	h.app.Editor = func(path string) error { return os.WriteFile(path, []byte("s2  two  /p/2\n"), 0o644) }
	h.must(t, "save", "--edit")
	h.at(time.Hour)
	h.hook(t, "stop", `{}`)
	if got := strings.Join(latestIDs(t, h, "default"), ","); got != "s2" {
		t.Fatalf("autosave re-added the session removed in the editor: %s", got)
	}
	// A plain save means "save what is running" and brings it back.
	h.must(t, "save")
	h.at(2 * time.Hour)
	h.hook(t, "stop", `{}`)
	if got := latestIDs(t, h, "default"); len(got) != 2 {
		t.Fatalf("after a plain save: %v", got)
	}
}

func TestInitKeepsThePristineBackup(t *testing.T) {
	h, p := initApp(t, userSettings)
	h.must(t, "init")
	h.app.Executable = "/usr/local/bin/ccshift"
	h.must(t, "init")
	if b, _ := os.ReadFile(p + ".ccshift-bak"); string(b) != userSettings {
		t.Fatalf("backup was overwritten with a file that already had ccshift in it:\n%s", b)
	}
}

func TestInitFollowsASymlinkedSettingsFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs symlinks or a POSIX shell")
	}
	h, p := initApp(t, "")
	real := filepath.Join(t.TempDir(), "dotfiles-settings.json")
	os.WriteFile(real, []byte(userSettings), 0o600)
	os.Symlink(real, p)
	h.must(t, "init")
	if fi, _ := os.Lstat(p); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "hook session-start") {
		t.Fatal("the link target was not updated")
	}
}

func TestInitRefusesWhenSettingsChangedUnderneath(t *testing.T) {
	_, p := initApp(t, userSettings)
	f, _ := settings.Load(p)
	f.AddHook("Stop", "x", 5)
	os.WriteFile(p, []byte(`{"theme": "light"}`), 0o600)
	if err := f.Save(); err == nil {
		t.Fatal("Save should refuse when the file changed after it was loaded")
	}
	if b, _ := os.ReadFile(p); string(b) != `{"theme": "light"}` {
		t.Fatal("the other writer's change was lost")
	}
}

func TestInitWithNullStatusLine(t *testing.T) {
	h, p := initApp(t, `{"statusLine": null}`)
	h.must(t, "init")
	f, _ := settings.Load(p)
	if !strings.Contains(string(f.StatusLine()), "statusline") {
		t.Fatalf("statusLine = %s", f.StatusLine())
	}
}

func TestInitWithARenamedBinary(t *testing.T) {
	h, p := initApp(t, userSettings)
	h.app.Executable = "/usr/local/bin/cs"
	h.must(t, "init")
	h.must(t, "init")
	st, _ := h.app.Store.InitState()
	if !strings.Contains(string(st.PreviousStatusLine), "x.js") {
		t.Fatalf("second init stored its own command as the previous statusline: %s", st.PreviousStatusLine)
	}
	f, _ := settings.Load(p)
	if got := f.HookCommands("SessionEnd"); len(got) != 1 {
		t.Fatalf("hooks duplicated: %q", got)
	}
	h.must(t, "init", "--remove")
	b, _ := os.ReadFile(p)
	if canonJSON(t, b) != canonJSON(t, []byte(userSettings)) {
		t.Fatalf("not restored:\n%s", b)
	}
}

func TestStatuslineDoesNotCallItself(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Store.SetInitState(store.InitState{PreviousStatusLine: json.RawMessage(`{"type":"command","command":"cs statusline"}`)})
	called := false
	h.app.Shell = func(context.Context, string, []byte) ([]byte, error) { called = true; return []byte("x"), nil }
	h.app.Env["CCSHIFT_STATUSLINE"] = "1"
	h.status(t, statusJSON("s1", "api", 10))
	if called {
		t.Fatal("a statusline started by ccshift must not run the previous command again")
	}
}

func TestRemoveWithLostStateFallsBackToTheBackup(t *testing.T) {
	h, p := initApp(t, userSettings)
	h.must(t, "init")
	os.Remove(filepath.Join(h.app.Store.Dir, "init.json"))
	h.must(t, "init", "--remove")
	f, _ := settings.Load(p)
	if !strings.Contains(string(f.StatusLine()), "x.js") {
		t.Fatalf("statusLine = %s", f.StatusLine())
	}
}

func TestResumedSessionLosesItsTombstoneEvenIfAutosaveFails(t *testing.T) {
	h := testApp(t, term.Exact)
	h.sessions = twoSessions()
	h.hook(t, "session-start", `{}`)
	h.hook(t, "session-end", `{"session_id":"s1","reason":"prompt_input_exit"}`)
	terms := h.app.Terms
	h.app.Terms = nil // makes the autosave view fail
	h.hook(t, "session-start", `{"session_id":"s1","source":"resume"}`)
	h.app.Terms = terms
	if _, still := h.app.Store.Ended()["s1"]; still {
		t.Fatal("tombstone should be cleared before autosave runs")
	}
}

func TestWarningHasHysteresis(t *testing.T) {
	h := testApp(t, term.Exact)
	var notes []string
	h.app.Notify = func(_, body string, _ []string) { notes = append(notes, body) }
	for _, pct := range []float64{72, 69, 71, 68, 73} {
		h.status(t, statusJSON("s1", "api", pct))
	}
	if len(notes) != 1 {
		t.Fatalf("hovering around the threshold notified %d times", len(notes))
	}
}

func TestPreviousStatuslineOutputIsShownEvenOnNonZeroExit(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Store.SetInitState(store.InitState{PreviousStatusLine: json.RawMessage(`{"type":"command","command":"x"}`)})
	h.app.Shell = func(context.Context, string, []byte) ([]byte, error) { return []byte("line\n"), os.ErrDeadlineExceeded }
	if out := h.status(t, statusJSON("s1", "api", 10)); out != "line\n" {
		t.Fatalf("out = %q", out)
	}
}

func TestRunShellKillsBackgroundChildrenAtTheDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs symlinks or a POSIX shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	runShell(ctx, "sleep 5 & sleep 5", nil)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("runShell waited %v for a grandchild", time.Since(start))
	}
}

func TestConcurrentHooksKeepEverySession(t *testing.T) {
	base := testApp(t, term.Exact)
	var all []liveSession
	for i := range 8 {
		id := "c" + string(rune('0'+i))
		all = append(all, liveSession{id: id, cwd: "/p/" + id, name: id, pid: 100 + i, minute: i})
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := testApp(t, term.Exact)
			h.app.Store = base.app.Store
			h.sessions = all
			h.app.In = strings.NewReader(`{"session_id":"` + all[i].id + `"}`)
			h.app.Run(context.Background(), []string{"hook", "session-start"})
		}()
	}
	wg.Wait()
	if got := latestIDs(t, base, "default"); len(got) != 8 {
		t.Fatalf("saved %d of 8 sessions: %v", len(got), got)
	}
}
