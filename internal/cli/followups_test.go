package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func TestSaveEditWithEveryLineRemoved(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{{id: "s1", cwd: "/p/1", name: "one", pid: 1}}
	h.app.Editor = func(path string) error { return os.WriteFile(path, []byte("# nothing\n"), 0o644) }
	out := h.must(t, "save", "--edit")
	if !strings.Contains(out, "default: every line was removed, nothing saved") || strings.Contains(out, "no sessions running") {
		t.Fatalf("out = %q", out)
	}
}

func TestCorruptOrderFileIsIgnoredWithAWarning(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	h.sessions = []liveSession{{id: "s1", cwd: "/p/1", name: "one", pid: 1}}
	os.WriteFile(filepath.Join(h.app.Store.Dir, "order.json"), []byte("{broken"), 0o600)
	out := h.must(t, "ls")
	if !strings.Contains(out, "one") || !strings.Contains(h.err.String(), "order.json") {
		t.Fatalf("out=%q err=%q", out, h.err)
	}
	h.must(t, "save")
	if o, err := h.app.Store.Order(); err != nil || len(o["default"]) != 1 {
		t.Fatalf("save should rewrite the order file: %v %v", o, err)
	}
}

func TestForgetWarnsAboutUnreadableSave(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	saveSnap(t, h, "broken", "s9")
	os.WriteFile(filepath.Join(h.app.Store.Dir, "workspaces", "broken", "latest.json"), []byte("{broken"), 0o600)
	h.must(t, "forget", "name s1")
	if !strings.Contains(h.err.String(), "broken") {
		t.Fatalf("stderr = %q", h.err)
	}
}

func TestRestoreMentionsBackgroundSessionsOnlyWhenTheyNeedRespawn(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1")
	h.bgStates = []string{"blocked", "running"}
	if out := h.must(t, "restore", "--dry-run"); strings.Contains(out, "respawn") {
		t.Fatalf("out = %q", out)
	}
	h.bgStates = []string{"blocked", "failed", "stopped"}
	if out := h.must(t, "restore", "--dry-run"); !strings.Contains(out, "2 background sessions stopped") || !strings.Contains(out, "claude respawn --all") {
		t.Fatalf("out = %q", out)
	}
}

func TestSaveNamesSessionsDroppedFromThePreviousSave(t *testing.T) {
	h := testApp(t, term.LaunchOnly)
	saveSnap(t, h, "default", "s1", "s2", "s3")
	h.sessions = []liveSession{{id: "s2", cwd: "/p/s2", name: "name s2", pid: 1}}
	out := h.must(t, "save")
	if !strings.Contains(out, "dropped 2 sessions that are no longer running: name s1, name s3") || !strings.Contains(out, "ccshift restore default --pick") {
		t.Fatalf("out = %q", out)
	}
	h.sessions = append(h.sessions, liveSession{id: "s4", cwd: "/p/s4", name: "four", pid: 2})
	if out := h.must(t, "save"); strings.Contains(out, "dropped") {
		t.Fatalf("out = %q", out)
	}
}

func TestRestoreSkipsDeletedDirectory(t *testing.T) {
	h := testApp(t, term.Exact)
	saveSnap(t, h, "work", "s1", "s2")
	h.app.DirExists = func(p string) bool { return p != "/p/s1" }
	out := h.must(t, "restore")
	if len(h.term.opened) != 1 || len(h.term.opened[0]) != 1 || h.term.opened[0][0].Argv[2] != "s2" {
		t.Fatalf("opened = %+v", h.term.opened)
	}
	if !strings.Contains(out, "skipped name s1: directory no longer exists: /p/s1") {
		t.Fatalf("out = %q", out)
	}
}
