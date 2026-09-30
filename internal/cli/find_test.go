package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timileyinpelumi/ccshift/internal/term"
)

func (h *harness) transcript(t *testing.T, id, cwd string, lines ...string) {
	t.Helper()
	dir := filepath.Join(h.projects, strings.ReplaceAll(cwd, "/", "-"))
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func TestFindAndResume(t *testing.T) {
	h := testApp(t, term.Exact)
	h.transcript(t, "old-1", "/w/api",
		`{"type":"user","cwd":"/w/api","message":{"content":"fix the auth bug in the login flow"}}`,
		`{"type":"custom-title","customTitle":"api · PAY-12"}`)
	h.transcript(t, "old-2", "/w/web", `{"type":"user","cwd":"/w/web","message":{"content":"unrelated"}}`)
	var dir string
	h.app.Chdir = func(d string) error { dir = d; return nil }

	out := h.must(t, "find", "auth", "bug")
	if !strings.Contains(out, "1  ") || !strings.Contains(out, "api · PAY-12") || !strings.Contains(out, "fix the auth bug") || strings.Contains(out, "unrelated") {
		t.Fatalf("out:\n%s", out)
	}
	h.must(t, "find", "auth", "bug", "--resume", "1")
	if dir != "/w/api" || len(h.execd) != 1 || strings.Join(h.execd[0], " ") != "/bin/claude --resume old-1" {
		t.Fatalf("dir=%q exec=%q", dir, h.execd)
	}
	if out := h.must(t, "find", "nothing-matches-this"); !strings.Contains(out, "No past session mentions") {
		t.Fatalf("out = %q", out)
	}
}

func TestFindSwitchesToARunningSession(t *testing.T) {
	h := testApp(t, term.Exact)
	h.term.tabs = []term.Tab{{ID: "p1"}}
	h.sessions = []liveSession{{id: "old-1", cwd: "/w/api", name: "api", pane: "p1", pid: 1}}
	h.transcript(t, "old-1", "/w/api", `{"type":"user","cwd":"/w/api","message":{"content":"the auth bug"}}`)
	out := h.must(t, "find", "auth", "--resume", "1")
	if len(h.execd) != 0 || len(h.term.focused) != 1 || !strings.Contains(out, "already running") {
		t.Fatalf("exec=%q focused=%v out=%q", h.execd, h.term.focused, out)
	}
}

func TestFindHere(t *testing.T) {
	h := testApp(t, term.Exact)
	h.app.Cwd = "/w/web"
	h.transcript(t, "a", "/w/api", `{"type":"user","cwd":"/w/api","message":{"content":"auth"}}`)
	h.transcript(t, "b", "/w/web/sub", `{"type":"user","cwd":"/w/web/sub","message":{"content":"auth"}}`)
	out := h.must(t, "find", "auth", "--here")
	if strings.Contains(out, "/w/api") || !strings.Contains(out, "/w/web/sub") {
		t.Fatalf("out:\n%s", out)
	}
}
