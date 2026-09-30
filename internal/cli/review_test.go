package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestRestoreOpensASessionOnceAcrossWorkspaces(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "default", "s1", "s2")
	saveSnap(t, h, "work", "s1")
	h.must(t, "restore")
	n := 0
	for _, w := range h.term.opened {
		for _, l := range w {
			if l.Argv[2] == "s1" {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("s1 opened %d times: %+v", n, h.term.opened)
	}
}

func TestRestorePrintsCommandsWhenTerminalFails(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	h.term.openErr = errors.New("remote control is disabled")
	if code := h.run("restore"); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(h.err.String(), "remote control is disabled") {
		t.Fatalf("stderr = %q", h.err)
	}
	if !strings.Contains(h.out.String(), "cd /p/s1 && /bin/claude --resume s1 '--name=name s1'") {
		t.Fatalf("stdout = %q", h.out)
	}
}

func TestSaveReportsWorkspacesWithNothingRunning(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Config.Workspaces = map[string][]string{"work": {"/w"}, "personal": {"/p"}}
	saveSnap(t, h, "personal", "s1", "s2")
	h.sessions = []liveSession{{id: "w1", cwd: "/w/api", name: "api", pid: 9}}
	out := h.must(t, "save")
	if !strings.Contains(out, "work: saved 1 sessions") {
		t.Fatalf("out = %q", out)
	}
	if !strings.Contains(out, "personal: no sessions running, kept the previous save (2 sessions)") || !strings.Contains(out, "ccshift forget") {
		t.Fatalf("out = %q", out)
	}
}

func TestSaveDoesNotHoldLockWhileEditing(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{{id: "s1", cwd: "/p/1", name: "one", pid: 1}}
	h.app.Editor = func(string) error {
		got := make(chan struct{})
		go func() {
			if unlock, err := h.app.Store.Lock(); err == nil {
				unlock()
			}
			close(got)
		}()
		select {
		case <-got:
			return nil
		case <-time.After(500 * time.Millisecond):
			return errors.New("store lock held while the editor is open")
		}
	}
	h.must(t, "save", "--edit")
}

func TestRestoreTakesLockToRecordOrder(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	saveSnap(t, h, "work", "s1")
	unlock, err := h.app.Store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan int)
	go func() { done <- h.run("restore") }()
	select {
	case <-done:
		unlock()
		t.Fatal("restore finished while the store lock was held")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	if code := <-done; code != 0 {
		t.Fatalf("exit = %d", code)
	}
}
